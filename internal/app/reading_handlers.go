package app
import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FB2RenderSection struct {
	ID         string   `json:"id"`
	Title      string   `json:"title,omitempty"`
	Paragraphs []string `json:"paragraphs,omitempty"`
}

type FB2RenderModel struct {
	Title      string           `json:"title"`
	Author     string           `json:"author,omitempty"`
	Language   string           `json:"language,omitempty"`
	CoverData  string           `json:"cover_data,omitempty"`
	TOC        []ReaderTOCItem  `json:"toc"`
	Sections   []FB2RenderSection `json:"sections"`
}

func findZipEntry(z *zip.ReadCloser, fileName, format string) *zip.File {
	targetName := fmt.Sprintf("%s.%s", fileName, format)
	for _, f := range z.File {
		if strings.EqualFold(f.Name, targetName) {
			return f
		}
	}
	for _, f := range z.File {
		if strings.EqualFold(f.Name, fileName) {
			return f
		}
	}
	return nil
}

func handleReaderMeta(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}
		fileName, zipName, format, title, language, author, del, err := dm.GetBookDownloadInfo(bookID)
		if err == sql.ErrNoRows {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Ошибка базы данных", http.StatusInternalServerError)
			return
		}
		if del == 1 {
			http.Error(w, "Книга удалена", http.StatusNotFound)
			return
		}
		format = ensureFormat(format, fileName)
		var toc []ReaderTOCItem
		zipPath := filepath.Join(booksDir, zipName)
		if zf, err := zip.OpenReader(zipPath); err == nil {
			if zfFile := findZipEntry(zf, fileName, format); zfFile != nil {
				if rc, err := zfFile.Open(); err == nil {
					if raw, err := io.ReadAll(rc); err == nil {
						toc = extractServerTOC(format, raw)
					}
					rc.Close()
				}
			}
			zf.Close()
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": bookID, "title": title, "author": author, "language": language,
			"format": format, "file_name": fileName, "zip": zipName,
			"toc_server": toc,
		})
	}
}

// formatNeedsFoliateConvertedProxy — форматы, которые foliate-js не открывает напрямую, но даём во встроенную
// читалку как FB2 после convertRawToFB2UTF8 (тот же конвейер, что для Liberama).
func formatNeedsFoliateConvertedProxy(format string, raw []byte) bool {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "txt", "html", "htm", "doc", "docx", "rtf", "djvu", "tcr", "odt", "htmlz", "rb", "pml", "pmlz":
		return true
	case "fb2":
		return !fb2LooksLikeFictionBook(raw)
	default:
		return false
	}
}

// handleReaderFoliateConvertedFB2 — отдаёт UTF-8 FictionBook 2.0 для vue-book-reader (URL оканчивается на .fb2).
func handleReaderFoliateConvertedFB2(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}
		fileName, _, format, title, _, author, del, err := dm.GetBookDownloadInfo(bookID)
		if err == sql.ErrNoRows || del == 1 {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Ошибка базы данных", http.StatusInternalServerError)
			return
		}
		format = ensureFormat(format, fileName)

		raw, _, err := extractBookBytes(dm, booksDir, bookID)
		if err != nil || len(raw) == 0 {
			http.Error(w, "Файл не найден", http.StatusNotFound)
			return
		}
		if int64(len(raw)) > maxConvertInputBytes {
			http.Error(w, fmt.Sprintf("Файл больше %d МБ", maxConvertInputBytes>>20), http.StatusRequestEntityTooLarge)
			return
		}
		if !formatNeedsFoliateConvertedProxy(format, raw) {
			http.Error(w, "Для этого формата используйте прямую ссылку на файл (/download/…)", http.StatusBadRequest)
			return
		}

		fb2out, err := convertRawToFB2UTF8(raw, format, title, author)
		if err != nil {
			log.Printf("foliate-converted.fb2 id=%d: %v", bookID, err)
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/fb2+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "private, max-age=120")
		_, _ = w.Write(fb2out)
	}
}

func handleBookPlainText(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}
		fileName, zipName, format, _, _, _, del, err := dm.GetBookDownloadInfo(bookID)
		if err != nil || del == 1 {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		format = ensureFormat(format, fileName)
		if format != "txt" {
			http.Error(w, "Формат не TXT", http.StatusBadRequest)
			return
		}
		zipPath := filepath.Join(booksDir, zipName)
		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			http.Error(w, "Архив недоступен", http.StatusInternalServerError)
			return
		}
		defer zf.Close()
		zfFile := findZipEntry(zf, fileName, format)
		if zfFile == nil {
			http.Error(w, "Файл не найден в архиве", http.StatusNotFound)
			return
		}
		rc, err := zfFile.Open()
		if err != nil {
			http.Error(w, "Не удалось прочитать файл", http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "private, max-age=60")
		_, _ = io.Copy(w, rc)
	}
}

func handleBookFB2Model(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}

		fileName, zipName, format, title, language, author, del, err := dm.GetBookDownloadInfo(bookID)
		if err != nil || del == 1 {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		format = ensureFormat(format, fileName)
		if format != "fb2" {
			http.Error(w, "Формат не FB2", http.StatusBadRequest)
			return
		}

		zipPath := filepath.Join(booksDir, zipName)
		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			http.Error(w, "Архив недоступен", http.StatusInternalServerError)
			return
		}
		defer zf.Close()
		zfFile := findZipEntry(zf, fileName, format)
		if zfFile == nil {
			http.Error(w, "Файл не найден в архиве", http.StatusNotFound)
			return
		}

		rc, err := zfFile.Open()
		if err != nil {
			http.Error(w, "Не удалось прочитать файл", http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			http.Error(w, "Не удалось прочитать содержимое", http.StatusInternalServerError)
			return
		}

		fb2, err := ParseFB2Metadata(raw)
		if err != nil {
			http.Error(w, "Некорректный FB2", http.StatusUnprocessableEntity)
			return
		}

		model := FB2RenderModel{
			Title:    firstNonEmpty(fb2.Description.TitleInfo.BookTitle, title),
			Author:   firstNonEmpty(firstFB2Author(fb2.Description.TitleInfo.Author), author),
			Language: firstNonEmpty(fb2.Description.TitleInfo.Lang, language),
			TOC:      extractServerTOC("fb2", raw),
			Sections: makeFB2Sections(fb2.Body.Section),
			CoverData: extractFB2CoverData(fb2),
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(model)
	}
}

func makeFB2Sections(in []Section) []FB2RenderSection {
	out := make([]FB2RenderSection, 0, len(in))
	for i, sec := range in {
		paras := make([]string, 0, len(sec.Paragraph))
		for _, p := range sec.Paragraph {
			if txt := cleanText(p); txt != "" {
				paras = append(paras, txt)
			}
		}
		if len(paras) == 0 {
			continue
		}
		out = append(out, FB2RenderSection{
			ID:         fmt.Sprintf("fb2-sec-%d", i+1),
			Title:      cleanText(sec.Title),
			Paragraphs: paras,
		})
	}
	return out
}

func docxCacheDir() (string, error) {
	dir := filepath.Join("config", "docx_cache")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func findLibreOfficeCLI() string {
	if p := strings.TrimSpace(os.Getenv("LIBREOFFICE_PATH")); p != "" {
		return p
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		path, err := exec.LookPath(name)
		if err == nil {
			return path
		}
	}
	return ""
}

func convertDocToDocx(srcPath, outDir string) error {
	cli := findLibreOfficeCLI()
	if cli == "" {
		return fmt.Errorf("LibreOffice не найден в PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli,
		"--headless", "--nologo", "--nofirststartwizard",
		"--convert-to", "docx", "--outdir", outDir, srcPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func handleBookDocxForReader(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}
		fileName, zipName, format, title, _, _, del, err := dm.GetBookDownloadInfo(bookID)
		if err != nil || del == 1 {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		format = ensureFormat(format, fileName)
		if format != "doc" && format != "docx" {
			http.Error(w, "Формат не DOC/DOCX", http.StatusBadRequest)
			return
		}
		zipPath := filepath.Join(booksDir, zipName)
		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			http.Error(w, "Архив недоступен", http.StatusInternalServerError)
			return
		}
		defer zf.Close()
		zfFile := findZipEntry(zf, fileName, format)
		if zfFile == nil {
			http.Error(w, "Файл не найден в архиве", http.StatusNotFound)
			return
		}

		if format == "docx" {
			rc, err := zfFile.Open()
			if err != nil {
				http.Error(w, "Не удалось прочитать файл", http.StatusInternalServerError)
				return
			}
			defer rc.Close()
			safeTitle := sanitizeFilename(title)
			if safeTitle == "" {
				safeTitle = "book"
			}
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.docx"`, safeTitle))
			_, _ = io.Copy(w, rc)
			return
		}

		cacheDir, err := docxCacheDir()
		if err != nil {
			log.Printf("docx cache: %v", err)
			http.Error(w, "Кэш недоступен", http.StatusInternalServerError)
			return
		}
		cached := filepath.Join(cacheDir, fmt.Sprintf("%d.docx", bookID))
		if st, err := os.Stat(cached); err == nil && st.Size() > 0 {
			http.ServeFile(w, r, cached)
			return
		}

		tmpDir, err := os.MkdirTemp("", "docconv-*")
		if err != nil {
			http.Error(w, "Временная папка", http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(tmpDir)
		srcPath := filepath.Join(tmpDir, "src.doc")
		outF, err := os.Create(srcPath)
		if err != nil {
			http.Error(w, "Запись временного файла", http.StatusInternalServerError)
			return
		}
		rc, err := zfFile.Open()
		if err != nil {
			outF.Close()
			http.Error(w, "Чтение DOC", http.StatusInternalServerError)
			return
		}
		_, err = io.Copy(outF, rc)
		rc.Close()
		outF.Close()
		if err != nil {
			http.Error(w, "Копирование DOC", http.StatusInternalServerError)
			return
		}

		if err := convertDocToDocx(srcPath, tmpDir); err != nil {
			log.Printf("Конвертация DOC→DOCX (книга %d): %v", bookID, err)
			http.Error(w, "Конвертация недоступна (установите LibreOffice)", http.StatusServiceUnavailable)
			return
		}
		converted := filepath.Join(tmpDir, "src.docx")
		if _, err := os.Stat(converted); err != nil {
			http.Error(w, "Результат конвертации не найден", http.StatusInternalServerError)
			return
		}
		if err := copyFile(converted, cached); err != nil {
			log.Printf("Кэш docx: %v", err)
			http.ServeFile(w, r, converted)
			return
		}
		http.ServeFile(w, r, cached)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func handleReadingProgressGet(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		claims := parseReadingProgressUserClaims(r, sm.Config.JWTSigningKey)
		bookID, err := strconv.Atoi(r.URL.Query().Get("book_id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный book_id", http.StatusBadRequest)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База недоступна", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if claims != nil {
			pos, has, err := dm.GetReadingProgress(claims.UserID, bookID)
			if err != nil {
				http.Error(w, "Ошибка чтения", http.StatusInternalServerError)
				return
			}
			if !has {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"position": nil})
				return
			}
			var parsed interface{}
			if err := json.Unmarshal([]byte(pos), &parsed); err != nil {
				parsed = pos
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"position": parsed})
			return
		}

		guestKey := guestReaderKeyFromRequest(r)
		if guestKey == "" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"position": nil})
			return
		}
		pos, has, err := dm.GetGuestReadingProgress(guestKey, bookID)
		if err != nil {
			http.Error(w, "Ошибка чтения", http.StatusInternalServerError)
			return
		}
		if !has {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"position": nil})
			return
		}
		var parsed2 interface{}
		if err := json.Unmarshal([]byte(pos), &parsed2); err != nil {
			parsed2 = pos
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"position": parsed2})
	}
}

func handleReadingProgressAPI(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleReadingProgressGet(sm)(w, r)
		case http.MethodPut, http.MethodPost:
			handleReadingProgressPut(sm)(w, r)
		default:
			w.Header().Set("Allow", "GET, PUT, POST")
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		}
	}
}

func handleReadingProgressPut(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		claims := parseReadingProgressUserClaims(r, sm.Config.JWTSigningKey)
		var req struct {
			BookID   int             `json:"book_id"`
			Position json.RawMessage `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный JSON", http.StatusBadRequest)
			return
		}
		if req.BookID <= 0 || len(req.Position) == 0 {
			http.Error(w, "book_id и position обязательны", http.StatusBadRequest)
			return
		}
		dm := sm.GetDB()
		if dm == nil {
			http.Error(w, "База недоступна", http.StatusServiceUnavailable)
			return
		}
		if claims != nil {
			if err := dm.UpsertReadingProgress(claims.UserID, req.BookID, string(req.Position)); err != nil {
				http.Error(w, "Ошибка сохранения", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		guestKey := guestReaderKeyFromRequest(r)
		if guestKey == "" {
			http.Error(w, "Для сохранения без входа укажите cookie или заголовок X-Books-Guest-Reader", http.StatusBadRequest)
			return
		}
		if err := dm.UpsertGuestReadingProgress(guestKey, req.BookID, string(req.Position)); err != nil {
			http.Error(w, "Ошибка сохранения", http.StatusInternalServerError)
			return
		}
		setGuestReaderCookie(w, guestKey)
		w.WriteHeader(http.StatusNoContent)
	}
}
