package app
import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func urlpkgParse(s string) (*url.URL, error) { return url.Parse(s) }

// -----------------------------------------------------------------------------
// Liberama compatibility layer (WS + minimal HTTP).
// This is intentionally narrow and mirrors Liberama 1.3.2 expectations.
// -----------------------------------------------------------------------------

const (
	liberamaUploadDir          = "uploads" // served at /upload/<sha256>
	liberamaMaxUploadSizeBytes = int64(10 * 1024 * 1024)
)

var liberamaUpgrader = websocket.Upgrader{
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Same-origin in typical deployments; allow all to avoid breaking embedded iframe.
		return true
	},
}

type liberamaWorkerState struct {
	State        string `json:"state,omitempty"` // start|finish|error
	Progress     int    `json:"progress,omitempty"`
	Error        string `json:"error,omitempty"`
	Path         string `json:"path,omitempty"`
	Size         int64  `json:"size,omitempty"`
	LastModified int64  `json:"lastModified,omitempty"`
}

type liberamaHub struct {
	mu      sync.RWMutex
	workers map[string]liberamaWorkerState
}

var wsHub = &liberamaHub{workers: map[string]liberamaWorkerState{}}

func (h *liberamaHub) setWorker(id string, st liberamaWorkerState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workers[id] = st
}

func (h *liberamaHub) getWorker(id string) (liberamaWorkerState, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st, ok := h.workers[id]
	return st, ok
}

// -----------------------------------------------------------------------------
// Reader storage (server-side KV), modeled after Liberama JembaReaderStorage.
// Table: reader_storage(id TEXT PRIMARY KEY, rev INTEGER, time INTEGER, data TEXT)
// -----------------------------------------------------------------------------

type readerStorageCache struct {
	mu   sync.Mutex
	seen map[string]struct {
		rev      int64
		identity string
		at       time.Time
	}
}

var rsCache = &readerStorageCache{seen: map[string]struct {
	rev      int64
	identity string
	at       time.Time
}{}}

func ensureReaderStorageTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS reader_storage (
  id   TEXT PRIMARY KEY,
  rev  INTEGER NOT NULL,
  time INTEGER NOT NULL,
  data TEXT NOT NULL
);
`)
	return err
}

type readerStorageReq struct {
	Action   string                       `json:"action"`
	Items    map[string]readerStorageItem `json:"items"`
	Identity string                       `json:"identity,omitempty"`
	Force    bool                         `json:"force,omitempty"`
}

type readerStorageItem struct {
	Rev  int64  `json:"rev"`
	Data string `json:"data,omitempty"`
}

type readerStorageResp struct {
	State string                       `json:"state"`
	Items map[string]readerStorageItem `json:"items,omitempty"`
	Error string                       `json:"error,omitempty"`
}

func readerStorageCheck(db *sql.DB, ids []string) (map[string]readerStorageItem, error) {
	out := map[string]readerStorageItem{}
	for _, id := range ids {
		var rev sql.NullInt64
		err := db.QueryRow(`SELECT rev FROM reader_storage WHERE id = ?`, id).Scan(&rev)
		if errors.Is(err, sql.ErrNoRows) {
			out[id] = readerStorageItem{Rev: 0}
			continue
		}
		if err != nil {
			return nil, err
		}
		if rev.Valid {
			out[id] = readerStorageItem{Rev: rev.Int64}
		} else {
			out[id] = readerStorageItem{Rev: 0}
		}
	}
	return out, nil
}

func readerStorageGet(db *sql.DB, ids []string) (map[string]readerStorageItem, error) {
	out := map[string]readerStorageItem{}
	for _, id := range ids {
		var rev sql.NullInt64
		var data sql.NullString
		err := db.QueryRow(`SELECT rev, data FROM reader_storage WHERE id = ?`, id).Scan(&rev, &data)
		if errors.Is(err, sql.ErrNoRows) {
			out[id] = readerStorageItem{Rev: 0, Data: ""}
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = readerStorageItem{Rev: rev.Int64, Data: data.String}
	}
	return out, nil
}

func readerStorageSet(db *sql.DB, req readerStorageReq) (readerStorageResp, error) {
	ids := make([]string, 0, len(req.Items))
	for id := range req.Items {
		ids = append(ids, id)
	}
	cur, err := readerStorageCheck(db, ids)
	if err != nil {
		return readerStorageResp{}, err
	}

	// identity/allowUpdate behavior similar to Liberama:
	// allow if force OR revDiff==1 OR (sameClient and revDiff in {0,1})
	rsCache.mu.Lock()
	defer rsCache.mu.Unlock()

	for id, it := range req.Items {
		if it.Data == "" && req.Action == "set" {
			// data can be empty string legitimately; don't reject
		}
		old := rsCache.seen[id]
		sameClient := req.Identity != "" && old.identity == req.Identity
		if req.Identity != "" && old.identity != req.Identity {
			old.identity = req.Identity
		}
		revDiff := it.Rev - cur[id].Rev
		allow := req.Force || revDiff == 1 || (sameClient && (revDiff == 0 || revDiff == 1))
		if !allow {
			// reject with current revs
			return readerStorageResp{State: "reject", Items: cur}, nil
		}
		rsCache.seen[id] = struct {
			rev      int64
			identity string
			at       time.Time
		}{rev: it.Rev, identity: old.identity, at: time.Now()}
	}

	tx, err := db.Begin()
	if err != nil {
		return readerStorageResp{}, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	for id, it := range req.Items {
		_, err := tx.Exec(`INSERT INTO reader_storage(id, rev, time, data) VALUES(?,?,?,?)
ON CONFLICT(id) DO UPDATE SET rev=excluded.rev, time=excluded.time, data=excluded.data`, id, it.Rev, now, it.Data)
		if err != nil {
			return readerStorageResp{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return readerStorageResp{}, err
	}

	return readerStorageResp{State: "success"}, nil
}

// -----------------------------------------------------------------------------
// Upload helpers
// -----------------------------------------------------------------------------

func ensureUploadDir() (string, error) {
	dir := liberamaUploadDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var reXMLDeclEncoding = regexp.MustCompile(`(?i)<\\?xml[^>]*encoding=['"]([^'"]+)['"]`)
var reDownloadID = regexp.MustCompile(`/download/(\d+)`)

func normalizeEncodingName(enc string) string {
	enc = strings.ToLower(strings.TrimSpace(enc))
	enc = strings.ReplaceAll(enc, "_", "-")
	enc = strings.ReplaceAll(enc, "windows", "windows") // keep stable
	switch enc {
	case "windows-1251", "win-1251", "cp1251":
		return "windows-1251"
	case "koi8-r":
		return "koi8-r"
	case "iso-8859-5":
		return "iso-8859-5"
	case "cp866", "ibm866", "866":
		return "cp866"
	case "mac-cyrillic", "x-mac-cyrillic":
		return "mac-cyrillic"
	case "maccyrillic":
		return "mac-cyrillic"
	case "utf-8", "utf8":
		return "utf-8"
	case "windows-1252", "cp1252", "win-1252":
		return "windows-1252"
	case "iso-8859-1", "iso8859-1", "latin-1", "latin1", "cp819":
		return "iso-8859-1"
	case "utf-16le", "utf-16-le":
		return "utf-16le"
	case "utf-16be", "utf-16-be":
		return "utf-16be"
	default:
		return enc
	}
}

func getEncodingLiteLikeLiberama(buf []byte) string {
	// Port of liberama-1.3.2/server/core/Reader/BookConverter/textUtils.getEncodingLite
	const lowerCase = 3
	const upperCase = 1

	type scoreRec struct {
		name string
		c    int
	}

	scores := map[string]int{
		"k": 0, // koi8-r
		"w": 0, // windows-1251
		"d": 0, // cp866
		"i": 0, // iso-8859-5
		"m": 0, // mac-cyrillic
		"u": 0, // utf-8
	}

	lenBuf := len(buf)
	blockSize := lenBuf
	if lenBuf > 5*3000 {
		blockSize = 3000
	}
	counter := 0
	i := 0
	totalChecked := 0
	for i < lenBuf {
		char := int(buf[i])
		nextChar := 0
		if i < lenBuf-1 {
			nextChar = int(buf[i+1])
		}
		totalChecked++
		i++

		// non-russian
		if char < 128 || char > 256 {
			continue
		}

		// UTF-8
		if (char == 208 || char == 209) && nextChar >= 128 && nextChar <= 190 {
			scores["u"] += lowerCase
		} else {
			// CP866
			if (char > 159 && char < 176) || (char > 223 && char < 242) {
				scores["d"] += lowerCase
			}
			if char > 127 && char < 160 {
				scores["d"] += upperCase
			}

			// KOI8-R
			if char > 191 && char < 223 {
				scores["k"] += lowerCase
			}
			if char > 222 && char < 256 {
				scores["k"] += upperCase
			}

			// WIN-1251
			if char > 223 && char < 256 {
				scores["w"] += lowerCase
			}
			if char > 191 && char < 224 {
				scores["w"] += upperCase
			}

			// MAC
			if char > 221 && char < 255 {
				scores["m"] += lowerCase
			}
			if char > 127 && char < 160 {
				scores["m"] += upperCase
			}

			// ISO-8859-5
			if char > 207 && char < 240 {
				scores["i"] += lowerCase
			}
			if char > 175 && char < 208 {
				scores["i"] += upperCase
			}
		}

		counter++
		if counter > blockSize {
			counter = 0
			i += int(float64(lenBuf)/2 - 2*float64(blockSize) + 0.5)
		}
	}

	codePage := map[string]string{
		"k": "koi8-r",
		"w": "windows-1251",
		"d": "cp866",
		"i": "iso-8859-5",
		"m": "mac-cyrillic",
		"u": "utf-8",
	}

	sorted := make([]scoreRec, 0, len(scores))
	for k, c := range scores {
		sorted = append(sorted, scoreRec{name: codePage[k], c: c})
	}
	// simple selection: find max
	best := sorted[0]
	for _, r := range sorted[1:] {
		if r.c > best.c {
			best = r
		}
	}

	if best.c > 0 && best.c > totalChecked/2 {
		return best.name
	}
	return "iso-8859-5"
}

func getEncodingLikeLiberama(buf []byte) string {
	// Mirror Liberama getEncoding(): start with lite, then if ISO-8859-5, use a heavier detector.
	selected := getEncodingLiteLikeLiberama(buf)
	if strings.EqualFold(selected, "iso-8859-5") && len(buf) > 10 {
		sample := buf
		if len(sample) > 20000 {
			sample = sample[:20000]
		}
		det := chardet.NewTextDetector()
		if res, err := det.DetectBest(sample); err == nil && res != nil {
			name := normalizeEncodingName(res.Charset)
			if name != "" && !strings.Contains(name, "iso-8859") {
				selected = name
			}
		}
	}
	return normalizeEncodingName(selected)
}

func replaceFB2XmlEncodingToUTF8(decoded []byte) []byte {
	// Mimic ConvertFb2.checkEncoding: replace encoding="..." with encoding="utf-8" after decode.
	// This helps some FB2 parsers treat the bytes consistently.
	s := string(decoded)
	if !strings.Contains(s, "<?xml") {
		return decoded
	}
	return []byte(reXMLDeclEncoding.ReplaceAllStringFunc(s, func(m string) string {
		// Replace only the encoding part inside the xml declaration line.
		return reXMLDeclEncoding.ReplaceAllString(m, `<?xml version="1.0" encoding="utf-8"`)
	}))
}

func decodeBytesToUTF8(raw []byte, format string) ([]byte, bool) {
	// Returns (utf8, converted)
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		return raw[3:], true
	}

	// UTF-16 BOM
	if len(raw) >= 2 {
		if raw[0] == 0xFF && raw[1] == 0xFE {
			out, err := io.ReadAll(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder().Reader(bytes.NewReader(raw)))
			if err == nil && len(out) > 0 {
				if format == "fb2" {
					out = replaceFB2XmlEncodingToUTF8(out)
				}
				return out, true
			}
		}
		if raw[0] == 0xFE && raw[1] == 0xFF {
			out, err := io.ReadAll(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder().Reader(bytes.NewReader(raw)))
			if err == nil && len(out) > 0 {
				if format == "fb2" {
					out = replaceFB2XmlEncodingToUTF8(out)
				}
				return out, true
			}
		}
	}

	enc := ""
	if format == "fb2" || format == "xml" || format == "html" || format == "htm" {
		// Try to sniff XML declaration (FB2) first
		head := raw
		if len(head) > 4096 {
			head = head[:4096]
		}
		if m := reXMLDeclEncoding.FindSubmatch(head); len(m) == 2 {
			enc = normalizeEncodingName(string(m[1]))
		}
	}
	enc = normalizeEncodingName(enc)

	// Special-case: FB2 bytes are already valid UTF-8, but XML header lies (e.g. windows-1251).
	// In that case, DON'T decode — just fix the header, otherwise downstream may mis-decode.
	if format == "fb2" && utf8.Valid(raw) && enc != "" && enc != "utf-8" {
		fixed := replaceFB2XmlEncodingToUTF8(raw)
		if !bytes.Equal(fixed, raw) {
			return fixed, true
		}
	}

	var dec func([]byte) ([]byte, error)
	switch enc {
	case "windows-1252":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.Windows1252.NewDecoder().Reader(bytes.NewReader(b))) }
	case "iso-8859-1":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.ISO8859_1.NewDecoder().Reader(bytes.NewReader(b))) }
	case "windows-1251":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.Windows1251.NewDecoder().Reader(bytes.NewReader(b))) }
	case "koi8-r":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.KOI8R.NewDecoder().Reader(bytes.NewReader(b))) }
	case "iso-8859-5":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.ISO8859_5.NewDecoder().Reader(bytes.NewReader(b))) }
	case "cp866":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.CodePage866.NewDecoder().Reader(bytes.NewReader(b))) }
	case "mac-cyrillic":
		dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.MacintoshCyrillic.NewDecoder().Reader(bytes.NewReader(b))) }
	case "utf-8", "":
		// Unknown or UTF-8: detect like Liberama and decode if needed.
		if format == "fb2" || format == "txt" || format == "html" || format == "htm" || format == "xml" {
			detected := getEncodingLikeLiberama(raw)
			if detected != "" && detected != "utf-8" {
				enc = detected
				// select decoder by encoding
				switch enc {
				case "windows-1251":
					dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.Windows1251.NewDecoder().Reader(bytes.NewReader(b))) }
				case "koi8-r":
					dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.KOI8R.NewDecoder().Reader(bytes.NewReader(b))) }
				case "iso-8859-5":
					dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.ISO8859_5.NewDecoder().Reader(bytes.NewReader(b))) }
				case "cp866":
					dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.CodePage866.NewDecoder().Reader(bytes.NewReader(b))) }
				case "mac-cyrillic":
					dec = func(b []byte) ([]byte, error) { return io.ReadAll(charmap.MacintoshCyrillic.NewDecoder().Reader(bytes.NewReader(b))) }
				default:
					return raw, false
				}
				out, err := dec(raw)
				if err == nil && len(out) > 0 {
					if format == "fb2" {
						out = replaceFB2XmlEncodingToUTF8(out)
					}
					return out, true
				}
			}
		}
		return raw, false
	default:
		// Unknown encoding: leave as-is
		return raw, false
	}
	out, err := dec(raw)
	if err != nil {
		return raw, false
	}
	if format == "fb2" {
		out = replaceFB2XmlEncodingToUTF8(out)
	}
	return out, true
}

func extractBookBytes(dm *DBManager, booksDir string, bookID int) (data []byte, format string, err error) {
	fileName, zipName, fmtDB, _, _, _, del, err := dm.GetBookDownloadInfo(bookID)
	if err != nil {
		return nil, "", err
	}
	if del == 1 {
		return nil, "", fmt.Errorf("book deleted")
	}
	format = ensureFormat(fmtDB, fileName)
	zipPath := filepath.Join(booksDir, zipName)
	zf, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer zf.Close()

	targetWithExt := fileName
	if format != "" && !strings.HasSuffix(strings.ToLower(fileName), "."+strings.ToLower(format)) {
		targetWithExt = fmt.Sprintf("%s.%s", fileName, format)
	}

	var target *zip.File
	for _, f := range zf.File {
		if strings.EqualFold(f.Name, targetWithExt) || strings.EqualFold(f.Name, fileName) {
			target = f
			break
		}
	}
	if target == nil {
		base1 := filepath.Base(targetWithExt)
		base2 := filepath.Base(fileName)
		for _, f := range zf.File {
			b := filepath.Base(f.Name)
			if strings.EqualFold(b, base1) || strings.EqualFold(b, base2) {
				target = f
				break
			}
		}
	}
	if target == nil {
		return nil, "", fmt.Errorf("file not found in zip")
	}

	rc, err := target.Open()
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}
	return raw, format, nil
}

func parseWSBuf(v any) ([]byte, error) {
	switch t := v.(type) {
	case string:
		// Some clients may send base64; try plain hex/base64 guess is risky, keep string as bytes.
		return []byte(t), nil
	case []any:
		out := make([]byte, 0, len(t))
		for _, x := range t {
			switch n := x.(type) {
			case float64:
				out = append(out, byte(int(n)))
			case int:
				out = append(out, byte(n))
			default:
				return nil, fmt.Errorf("unsupported buf item type")
			}
		}
		return out, nil
	case map[string]any:
		// Node Buffer JSON {type:'Buffer', data:[...]} or typed array object with numeric keys.
		if data, ok := t["data"]; ok {
			if arr, ok := data.([]any); ok {
				return parseWSBuf(arr)
			}
		}
		// numeric keys
		if ln, ok := t["length"].(float64); ok && ln > 0 {
			out := make([]byte, int(ln))
			for i := 0; i < int(ln); i++ {
				if vv, ok := t[strconv.Itoa(i)].(float64); ok {
					out[i] = byte(int(vv))
				}
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("unsupported buf type")
}

// -----------------------------------------------------------------------------
// HTTP: POST /api/reader/upload-file
// -----------------------------------------------------------------------------

func handleLiberamaUploadFile(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(liberamaMaxUploadSizeBytes); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Некорректный multipart"})
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Файл не найден"})
			return
		}
		defer f.Close()

		// read up to limit
		var buf bytes.Buffer
		if _, err := io.CopyN(&buf, f, liberamaMaxUploadSizeBytes+1); err != nil && !errors.Is(err, io.EOF) {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Ошибка чтения файла"})
			return
		}
		if int64(buf.Len()) > liberamaMaxUploadSizeBytes {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Размер файла превышает лимит"})
			return
		}

		dir, err := ensureUploadDir()
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Не удалось создать папку upload"})
			return
		}
		hash := sha256Hex(buf.Bytes())
		out := filepath.Join(dir, hash)
		if _, err := os.Stat(out); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
				_ = json.NewEncoder(w).Encode(map[string]any{"state": "error", "error": "Не удалось сохранить файл"})
				return
			}
		} else {
			// touch
			_ = os.Chtimes(out, time.Now(), time.Now())
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "success", "url": "disk://" + hash})
	}
}

// -----------------------------------------------------------------------------
// WebSocket /ws (and /liberama/ws alias)
// -----------------------------------------------------------------------------

type liberamaWSReq struct {
	RequestID int64           `json:"requestId"`
	Action    string          `json:"action"`
	Params    []string        `json:"params,omitempty"`
	ConfigHash string         `json:"_configHash,omitempty"`
	URL       string          `json:"url,omitempty"`
	WorkerID  string          `json:"workerId,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Buf       any             `json:"buf,omitempty"`
	BookUrls  []string        `json:"bookUrls,omitempty"`

	// passthrough flags for load-book (ignored for now)
	EnableSitesFilter *bool `json:"enableSitesFilter,omitempty"`
	SkipHtmlCheck     *bool `json:"skipHtmlCheck,omitempty"`
	IsText            *bool `json:"isText,omitempty"`
	UploadFileName    string `json:"uploadFileName,omitempty"`
}

func wsSend(conn *websocket.Conn, req liberamaWSReq, payload any) {
	var out map[string]any
	switch t := payload.(type) {
	case map[string]any:
		out = t
	default:
		out = map[string]any{}
		b, _ := json.Marshal(payload)
		_ = json.Unmarshal(b, &out)
	}
	if req.RequestID != 0 {
		out["requestId"] = req.RequestID
	}
	_ = conn.WriteJSON(out)
}

func handleLiberamaWS(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := liberamaUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req liberamaWSReq
			if err := json.Unmarshal(data, &req); err != nil {
				wsSend(conn, liberamaWSReq{}, map[string]any{"error": "bad json"})
				continue
			}

			// ack for WebSocketConnection.send (all non-ping)
			if req.Action != "_ping" {
				wsSend(conn, req, map[string]any{"_rok": 1})
			}

			switch req.Action {
			case "_ping":
				wsSend(conn, req, map[string]any{"_pong": true})

			case "get-config":
				cfg := map[string]any{
					"name":                 "Liberama",
					"version":              "1.3.2",
					"mode":                 "liberama",
					// Liberama is served under /liberama/, but static uploads are at /upload/.
					// Keep this empty so `${window.rootPathStatic}/upload/...` becomes `/upload/...`.
					"rootPathStatic":        "",
					"maxUploadFileSize":     liberamaMaxUploadSizeBytes,
					"useExternalBookConverter": false,
					"acceptFileExt":         "fb2,epub,txt,html,htm,doc,docx,rtf,pdf,mobi,azw,azw3,prc,djvu,cbz",
					"bucEnabled":            true,
					"branch":                "production",
					"networkLibraryLink":    "",
					"restricted":            map[string]any{},
					"donation":              map[string]any{},
				}
				// Only send requested params (and always include _configHash).
				out := map[string]any{}
				paramSet := map[string]struct{}{}
				for _, p := range req.Params {
					paramSet[p] = struct{}{}
				}
				for k, v := range cfg {
					if _, ok := paramSet[k]; ok {
						out[k] = v
					}
				}
				// simplistic config hash
				raw, _ := json.Marshal(out)
				sum := sha256.Sum256(raw)
				out["_configHash"] = hex.EncodeToString(sum[:])
				out["_useCached"] = false
				wsSend(conn, req, out)

			case "load-book":
				workerID := fmt.Sprintf("w_%d", time.Now().UnixNano())
				nowMs := time.Now().UnixMilli()

				url := strings.TrimSpace(req.URL)
				var path string
				switch {
				case strings.HasPrefix(url, "disk://"):
					path = "/upload/" + strings.TrimPrefix(url, "disk://")
				case strings.Contains(url, "/download/"):
					path = url
				case strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://"):
					if pu, err := urlpkgParse(url); err == nil {
						host := strings.ToLower(pu.Host)
						reqHost := strings.ToLower(r.Host)
						if host == reqHost && pu.Scheme != "" {
							path = pu.RequestURI()
						} else {
							path = pu.String()
						}
					} else {
						path = url
					}
				default:
					st := liberamaWorkerState{State: "error", Error: "Некорректный url", LastModified: nowMs}
					wsHub.setWorker(workerID, st)
					wsSend(conn, req, map[string]any{"workerId": workerID, "state": st.State, "error": st.Error, "lastModified": st.LastModified})
					continue
				}

				// Каталог: /download/<id>/... → FB2 (нормализация) или встроенная конвертация → FB2 в /upload/
				if strings.Contains(path, "/download/") {
					id := 0
					if m := reDownloadID.FindStringSubmatch(path); len(m) == 2 {
						if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
							id = n
						}
					}
					if id > 0 {
						dm := sm.GetDB()
						sm.Mu.RLock()
						booksDir := ""
						if sm.Config != nil {
							booksDir = sm.Config.BooksDir
						}
						sm.Mu.RUnlock()
						if dm != nil && booksDir != "" {
							bookTitle, bookAuthor := "", ""
							if _, _, _, bt, _, ba, del, ierr := dm.GetBookDownloadInfo(id); ierr == nil && del != 1 {
								bookTitle, bookAuthor = bt, ba
							}
							raw, fmtName, err := extractBookBytes(dm, booksDir, id)
							if err == nil && len(raw) > 0 {
								// PDF: отдаём оригинал в Liberama без конвертации в FB2.
								if strings.EqualFold(strings.TrimSpace(fmtName), "pdf") {
									st := liberamaWorkerState{
										State:        "finish",
										Progress:     100,
										Path:         path,
										Size:         int64(len(raw)),
										LastModified: time.Now().UnixMilli(),
									}
									wsHub.setWorker(workerID, st)
									wsSend(conn, req, map[string]any{
										"workerId":     workerID,
										"state":        st.State,
										"progress":     st.Progress,
										"path":         st.Path,
										"size":         st.Size,
										"lastModified": st.LastModified,
									})
									continue
								}
								if int64(len(raw)) > maxConvertInputBytes {
									st := liberamaWorkerState{State: "error", Error: fmt.Sprintf("Файл больше %d МБ, конвертация отключена", maxConvertInputBytes>>20), LastModified: time.Now().UnixMilli()}
									wsHub.setWorker(workerID, st)
									wsSend(conn, req, map[string]any{"workerId": workerID, "state": st.State, "error": st.Error, "lastModified": st.LastModified})
									continue
								}
								dir, derr := ensureUploadDir()
								if derr != nil {
									st := liberamaWorkerState{State: "error", Error: "Не удалось создать каталог upload", LastModified: time.Now().UnixMilli()}
									wsHub.setWorker(workerID, st)
									wsSend(conn, req, map[string]any{"workerId": workerID, "state": st.State, "error": st.Error, "lastModified": st.LastModified})
									continue
								}

								// FB2 FictionBook: только нормализация кодировки → /upload/<sha256>
								if strings.EqualFold(strings.TrimSpace(fmtName), "fb2") && fb2LooksLikeFictionBook(raw) {
									utf8Bytes, _ := decodeBytesToUTF8(raw, "fb2")
									hash := sha256Hex(utf8Bytes)
									out := filepath.Join(dir, hash)
									if _, err := os.Stat(out); errors.Is(err, os.ErrNotExist) {
										_ = os.WriteFile(out, utf8Bytes, 0o644)
									} else {
										_ = os.Chtimes(out, time.Now(), time.Now())
									}
									path = "/upload/" + hash
								} else {
									cacheKey := nativeConvertCacheKey(raw, fmtName)
									cacheFile := filepath.Join(dir, cacheKey)
									if fi, serr := os.Stat(cacheFile); serr == nil && fi.Size() > 0 {
										path = "/upload/" + cacheKey
									} else {
										wsHub.setWorker(workerID, liberamaWorkerState{State: "start", Progress: 0, LastModified: time.Now().UnixMilli()})
										wsSend(conn, req, map[string]any{
											"workerId": workerID, "state": "start", "progress": 0, "lastModified": time.Now().UnixMilli(),
										})
										ck := cacheKey
										fmts := fmtName
										rbytes := make([]byte, len(raw))
										copy(rbytes, raw)
										wid := workerID
										bt, ba := bookTitle, bookAuthor
										go func() {
											wsHub.setWorker(wid, liberamaWorkerState{State: "convert", Progress: 20, LastModified: time.Now().UnixMilli()})
											fb2raw, err := convertRawToFB2UTF8(rbytes, fmts, bt, ba)
											if err != nil {
												wsHub.setWorker(wid, liberamaWorkerState{State: "error", Error: err.Error(), LastModified: time.Now().UnixMilli()})
												return
											}
											wsHub.setWorker(wid, liberamaWorkerState{State: "convert", Progress: 75, LastModified: time.Now().UnixMilli()})
											utf8Bytes, _ := decodeBytesToUTF8(fb2raw, "fb2")
											upDir, err := ensureUploadDir()
											if err != nil {
												wsHub.setWorker(wid, liberamaWorkerState{State: "error", Error: err.Error(), LastModified: time.Now().UnixMilli()})
												return
											}
											dest := filepath.Join(upDir, ck)
											if err := os.WriteFile(dest, utf8Bytes, 0o644); err != nil {
												wsHub.setWorker(wid, liberamaWorkerState{State: "error", Error: err.Error(), LastModified: time.Now().UnixMilli()})
												return
											}
											fi, _ := os.Stat(dest)
											sz := int64(0)
											if fi != nil {
												sz = fi.Size()
											}
											wsHub.setWorker(wid, liberamaWorkerState{
												State: "finish", Progress: 100, Path: "/upload/" + ck, Size: sz, LastModified: time.Now().UnixMilli(),
											})
										}()
										continue
									}
								}
							}
						}
					}
				}

				var finSize int64
				if strings.HasPrefix(path, "/upload/") {
					if up, err := ensureUploadDir(); err == nil {
						base := strings.TrimPrefix(path, "/upload/")
						if fi, err := os.Stat(filepath.Join(up, base)); err == nil {
							finSize = fi.Size()
						}
					}
				}
				st := liberamaWorkerState{
					State:        "finish",
					Progress:     100,
					Path:         path,
					Size:         finSize,
					LastModified: time.Now().UnixMilli(),
				}
				wsHub.setWorker(workerID, st)
				wsSend(conn, req, map[string]any{
					"workerId":     workerID,
					"state":        st.State,
					"progress":     st.Progress,
					"path":         st.Path,
					"size":         st.Size,
					"lastModified": st.LastModified,
				})

			case "worker-get-state-finish":
				if req.WorkerID == "" {
					wsSend(conn, req, map[string]any{"state": "error", "error": "workerId is empty"})
					continue
				}
				st, ok := wsHub.getWorker(req.WorkerID)
				if !ok {
					// finish loop: send empty object per Liberama server if unknown
					wsSend(conn, req, map[string]any{})
					continue
				}
				wsSend(conn, req, map[string]any{
					"state":        st.State,
					"progress":     st.Progress,
					"error":        st.Error,
					"path":         st.Path,
					"size":         st.Size,
					"lastModified": st.LastModified,
				})

			case "reader-storage":
				dm := sm.GetDB()
				if dm == nil {
					wsSend(conn, req, map[string]any{"error": "DB not ready"})
					continue
				}
				if err := ensureReaderStorageTable(dm.DB()); err != nil {
					wsSend(conn, req, map[string]any{"error": "storage init failed"})
					continue
				}
				var rsReq readerStorageReq
				if err := json.Unmarshal(req.Body, &rsReq); err != nil {
					wsSend(conn, req, map[string]any{"error": "bad body"})
					continue
				}
				if rsReq.Action == "" {
					wsSend(conn, req, map[string]any{"error": "action is empty"})
					continue
				}
				if rsReq.Items == nil {
					wsSend(conn, req, map[string]any{"error": "items is empty"})
					continue
				}
				ids := make([]string, 0, len(rsReq.Items))
				for id := range rsReq.Items {
					ids = append(ids, id)
				}
				switch rsReq.Action {
				case "check":
					items, err := readerStorageCheck(dm.DB(), ids)
					if err != nil {
						wsSend(conn, req, map[string]any{"error": err.Error()})
						continue
					}
					wsSend(conn, req, readerStorageResp{State: "success", Items: items})
				case "get":
					items, err := readerStorageGet(dm.DB(), ids)
					if err != nil {
						wsSend(conn, req, map[string]any{"error": err.Error()})
						continue
					}
					wsSend(conn, req, readerStorageResp{State: "success", Items: items})
				case "set":
					resp, err := readerStorageSet(dm.DB(), rsReq)
					if err != nil {
						wsSend(conn, req, map[string]any{"error": err.Error()})
						continue
					}
					wsSend(conn, req, resp)
				default:
					wsSend(conn, req, map[string]any{"error": "Unknown action"})
				}

			case "upload-file-buf":
				b, err := parseWSBuf(req.Buf)
				if err != nil {
					wsSend(conn, req, map[string]any{"error": "bad buf"})
					continue
				}
				dir, err := ensureUploadDir()
				if err != nil {
					wsSend(conn, req, map[string]any{"error": "upload dir error"})
					continue
				}
				hash := sha256Hex(b)
				out := filepath.Join(dir, hash)
				if _, err := os.Stat(out); errors.Is(err, os.ErrNotExist) {
					if err := os.WriteFile(out, b, 0o644); err != nil {
						wsSend(conn, req, map[string]any{"error": "write error"})
						continue
					}
				} else {
					_ = os.Chtimes(out, time.Now(), time.Now())
				}
				wsSend(conn, req, map[string]any{"url": "disk://" + hash})

			case "upload-file-touch":
				if req.URL == "" || !strings.HasPrefix(req.URL, "disk://") {
					wsSend(conn, req, map[string]any{"error": "bad url"})
					continue
				}
				dir, err := ensureUploadDir()
				if err != nil {
					wsSend(conn, req, map[string]any{"error": "upload dir error"})
					continue
				}
				hash := strings.TrimPrefix(req.URL, "disk://")
				out := filepath.Join(dir, hash)
				_ = os.Chtimes(out, time.Now(), time.Now())
				wsSend(conn, req, map[string]any{"url": req.URL})

			case "check-buc":
				if req.BookUrls == nil {
					wsSend(conn, req, map[string]any{"error": "bookUrls is empty"})
					continue
				}
				type bucItem struct {
					ID   string `json:"id"`
					Size int64  `json:"size"`
				}
				resp := make([]bucItem, 0, len(req.BookUrls))
				for _, u := range req.BookUrls {
					u = strings.TrimSpace(u)
					size := int64(0)
					// Only support /download URLs (same-origin or relative)
					if strings.Contains(u, "/download/") {
						// Best-effort: if it's a local /download/<id>/..., we can query by id to get uncompressed size quickly.
						parts := strings.Split(strings.Trim(u, "/"), "/")
						if len(parts) >= 2 && parts[0] == "download" {
							if id, err := strconv.Atoi(parts[1]); err == nil && id > 0 {
								dm := sm.GetDB()
								if dm != nil {
									_, _, _, _, _, _, del, err := dm.GetBookDownloadInfo(id)
									if err == nil && del == 0 {
										// We don't have exact file size without opening zip entry; return 1 to indicate present.
										size = 1
									}
								}
							}
						}
					}
					resp = append(resp, bucItem{ID: u, Size: size})
				}
				wsSend(conn, req, map[string]any{"state": "success", "data": resp})

			default:
				wsSend(conn, req, map[string]any{"error": "Action not found: " + req.Action})
			}
		}
	}
}

// -----------------------------------------------------------------------------
// Static upload server: /upload/<sha256>
// -----------------------------------------------------------------------------

func handleUploadStatic() (http.Handler, error) {
	dir, err := ensureUploadDir()
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.Dir(dir)), nil
}

