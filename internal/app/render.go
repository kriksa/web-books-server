package app
import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"bytes"
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"web_books/internal/formats/docx"
	epubpkg "web_books/internal/formats/epub"
)

type ReaderRenderSection struct {
	ID            string                `json:"id"`
	Title         string                `json:"title,omitempty"`
	Paragraphs    []string              `json:"paragraphs,omitempty"`
	Footnotes     []string              `json:"footnotes,omitempty"`
	Refs          []ReaderRenderRefLink `json:"refs,omitempty"`
	FootnoteItems []ReaderRenderFootnote `json:"footnote_items,omitempty"`
	Anchors       []ReaderRenderAnchor  `json:"anchors,omitempty"`
}

type ReaderRenderRefLink struct {
	ID             string `json:"id"`
	NoteID         string `json:"note_id"`
	Label          string `json:"label,omitempty"`
	ParagraphIndex int   `json:"paragraph_index"`
}

type ReaderRenderFootnote struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	BackRefID string `json:"back_ref_id,omitempty"`
}

type ReaderRenderPage struct {
	Index           int     `json:"index"`
	SectionID       string  `json:"section_id"`
	ParagraphFrom   int     `json:"paragraph_from"`
	ParagraphTo     int     `json:"paragraph_to"`
	Fraction        float64 `json:"fraction"`
	PrimaryAnchorID string  `json:"primary_anchor_id,omitempty"`
}

type ReaderRenderAnchor struct {
	ID             string `json:"id"`
	Label          string `json:"label,omitempty"`
	ParagraphIndex int    `json:"paragraph_index"`
	Kind           string `json:"kind,omitempty"`
}

type ReaderViewportProfile struct {
	Width      int
	Height     int
	FontSize   float64
	LineHeight float64
	Margin     int
	Orientation string
	Mode        string
}

type ReaderRenderModel struct {
	ModelVersion int                  `json:"model_version"`
	Features     []string             `json:"features"`
	Format       string               `json:"format"`
	Title        string               `json:"title"`
	Author       string               `json:"author,omitempty"`
	Language     string               `json:"language,omitempty"`
	CoverData    string               `json:"cover_data,omitempty"`
	TOC          []ReaderTOCItem      `json:"toc"`
	Sections     []ReaderRenderSection `json:"sections"`
	Pages        []ReaderRenderPage   `json:"pages,omitempty"`
}

type repaginateRequest struct {
	Sections []ReaderRenderSection `json:"sections"`
}

type repaginateResponse struct {
	Pages []ReaderRenderPage `json:"pages"`
}

func handleBookRenderModel(dm *DBManager, booksDir string) http.HandlerFunc {
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
		if err == sql.ErrNoRows || del == 1 {
			http.Error(w, "Книга не найдена", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Ошибка базы данных", http.StatusInternalServerError)
			return
		}
		format = ensureFormat(format, fileName)

		if format != "fb2" && format != "epub" && format != "docx" && format != "txt" && format != "html" && format != "htm" && format != "md" {
			http.Error(w, "Формат не поддерживается render-model", http.StatusBadRequest)
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
			http.Error(w, "Не удалось открыть файл", http.StatusInternalServerError)
			return
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			http.Error(w, "Не удалось прочитать файл", http.StatusInternalServerError)
			return
		}

		model := ReaderRenderModel{
			ModelVersion: 1,
			Features:     []string{"sections", "toc", "anchors", "pages", "footnotes", "refs"},
			Format:       format,
			Title:        title,
			Author:       author,
			Language:     language,
			TOC:          nil,
		}

		start := time.Now()
		switch format {
		case "fb2":
			if err := fillFB2Model(raw, &model); err != nil {
				http.Error(w, "Ошибка FB2 парсинга", http.StatusUnprocessableEntity)
				return
			}
		case "docx":
			if err := fillDocxModel(raw, &model); err != nil {
				http.Error(w, "Ошибка DOCX парсинга", http.StatusUnprocessableEntity)
				return
			}
		case "epub":
			if err := fillEpubModel(raw, &model); err != nil {
				http.Error(w, "Ошибка EPUB парсинга", http.StatusUnprocessableEntity)
				return
			}
		case "txt":
			fillTxtModel(raw, &model)
		case "html", "htm":
			fillHTMLModel(raw, &model)
		case "md":
			fillMarkdownModel(raw, &model)
		}

		if strings.TrimSpace(model.Title) == "" {
			model.Title = fileName
		}
		vp := parseViewportProfile(r)
		model.Pages = paginateRenderSections(model.Sections, vp)
		if len(model.TOC) == 0 {
			model.TOC = tocFromSections(model.Sections)
		}
		log.Printf("render-model: id=%d format=%s sections=%d pages=%d duration_ms=%d bytes=%d", bookID, format, len(model.Sections), len(model.Pages), time.Since(start).Milliseconds(), len(raw))

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(model)
	}
}

func handleRepaginateModel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		var req repaginateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Некорректный JSON", http.StatusBadRequest)
			return
		}
		if len(req.Sections) == 0 {
			http.Error(w, "Пустые sections", http.StatusBadRequest)
			return
		}
		vp := parseViewportProfile(r)
		pages := paginateRenderSections(req.Sections, vp)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(repaginateResponse{Pages: pages})
	}
}

func fillTxtModel(raw []byte, model *ReaderRenderModel) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	chunks := strings.Split(text, "\n\n")
	pars := make([]string, 0, len(chunks))
	for _, c := range chunks {
		t := cleanText(c)
		if t != "" {
			pars = append(pars, t)
		}
	}
	model.Sections = []ReaderRenderSection{{
		ID:         "txt-1",
		Title:      model.Title,
		Paragraphs: pars,
		Anchors: []ReaderRenderAnchor{{
			ID:             "txt-anchor-1",
			Label:          model.Title,
			ParagraphIndex: 0,
			Kind:           "section",
		}},
	}}
}

func fillHTMLModel(raw []byte, model *ReaderRenderModel) {
	title, paragraphs, _, refs := extractXhtmlText(raw, "html-sec-1")
	if title != "" {
		model.Title = title
	}
	model.Sections = []ReaderRenderSection{{
		ID:         "html-sec-1",
		Title:      model.Title,
		Paragraphs: paragraphs,
		Refs:       refs,
		Anchors: []ReaderRenderAnchor{{
			ID:             "html-anchor-1",
			Label:          model.Title,
			ParagraphIndex: 0,
			Kind:           "section",
		}},
	}}
}

func fillMarkdownModel(raw []byte, model *ReaderRenderModel) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	sections := make([]ReaderRenderSection, 0, 8)
	cur := ReaderRenderSection{ID: "md-sec-1", Title: model.Title}
	secNum := 1
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "#") {
			if len(cur.Paragraphs) > 0 {
				sections = append(sections, cur)
				secNum++
			}
			cur = ReaderRenderSection{
				ID:    fmt.Sprintf("md-sec-%d", secNum),
				Title: cleanText(strings.TrimLeft(ln, "# ")),
				Anchors: []ReaderRenderAnchor{{
					ID:             fmt.Sprintf("md-sec-%d-anchor", secNum),
					Label:          cleanText(strings.TrimLeft(ln, "# ")),
					ParagraphIndex: 0,
					Kind:           "heading",
				}},
			}
			continue
		}
		if t := cleanText(ln); t != "" {
			cur.Paragraphs = append(cur.Paragraphs, t)
		}
	}
	if len(cur.Paragraphs) > 0 {
		sections = append(sections, cur)
	}
	model.Sections = sections
}

func fillFB2Model(raw []byte, model *ReaderRenderModel) error {
	fb2, err := ParseFB2Metadata(raw)
	if err != nil {
		return err
	}
	if t := strings.TrimSpace(fb2.Description.TitleInfo.BookTitle); t != "" {
		model.Title = t
	}
	if a := firstFB2Author(fb2.Description.TitleInfo.Author); a != "" {
		model.Author = a
	}
	if l := strings.TrimSpace(fb2.Description.TitleInfo.Lang); l != "" {
		model.Language = l
	}
	model.CoverData = extractFB2CoverData(fb2)

	sections, toc := parseFB2Structure(raw)
	enrichFB2FootnoteLinkage(raw, &sections)
	model.Sections = sections
	if len(toc) > 0 {
		model.TOC = toc
	}
	return nil
}

// enrichFB2FootnoteLinkage дополняет FootnoteItems по <body name="notes"> из XML:
// потоковый парсер иногда не совпадает по id с l:href — regex даёт полную карту сносок.
func enrichFB2FootnoteLinkage(raw []byte, sections *[]ReaderRenderSection) {
	if len(*sections) == 0 {
		return
	}
	noteMap := extractFB2NotesBodyMap(string(raw))
	if len(noteMap) == 0 {
		return
	}
	for si := range *sections {
		sec := &(*sections)[si]
		have := map[string]bool{}
		for _, f := range sec.FootnoteItems {
			if f.ID != "" {
				have[f.ID] = true
			}
		}
		for _, r := range sec.Refs {
			if r.NoteID == "" || have[r.NoteID] {
				continue
			}
			rawID := strings.TrimPrefix(r.NoteID, sec.ID+":")
			txt := resolveFB2NoteMapText(noteMap, rawID)
			if txt == "" {
				continue
			}
			sec.FootnoteItems = append(sec.FootnoteItems, ReaderRenderFootnote{
				ID:        r.NoteID,
				Text:      txt,
				BackRefID: r.ID,
			})
			have[r.NoteID] = true
		}
	}
}

func extractFB2NotesBodyMap(src string) map[string]string {
	out := map[string]string{}
	noteBody := regexp.MustCompile(`(?is)<body[^>]*name=["']notes["'][^>]*>(.*?)</body>`).FindStringSubmatch(src)
	if len(noteBody) < 2 {
		return out
	}
	secRe := regexp.MustCompile(`(?is)<section[^>]*id=["']([^"']+)["'][^>]*>(.*?)</section>`)
	for _, sm := range secRe.FindAllStringSubmatch(noteBody[1], -1) {
		k := strings.TrimSpace(sm[1])
		if k == "" {
			continue
		}
		out[k] = cleanText(stripTags(sm[2]))
	}
	return out
}

func resolveFB2NoteMapText(noteMap map[string]string, rawID string) string {
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return ""
	}
	if t := noteMap[rawID]; t != "" {
		return t
	}
	cands := []string{
		strings.TrimPrefix(rawID, "n_"),
		strings.TrimPrefix(rawID, "fn"),
		strings.TrimPrefix(rawID, "footnote"),
		strings.TrimPrefix(rawID, "#"),
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if t := noteMap[c]; t != "" {
			return t
		}
	}
	for k, v := range noteMap {
		if strings.EqualFold(k, rawID) {
			return v
		}
		if strings.HasSuffix(strings.ToLower(k), strings.ToLower(rawID)) {
			return v
		}
		if strings.HasSuffix(strings.ToLower(rawID), strings.ToLower(k)) {
			return v
		}
	}
	return ""
}

func fillDocxModel(raw []byte, model *ReaderRenderModel) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	docXML, err := docx.OpenDocumentXML(zr)
	if err != nil {
		return err
	}
	footnoteMap := parseDocxFootnotesMap(zr)
	dec := xml.NewDecoder(bytes.NewReader(docXML))
	sections := make([]ReaderRenderSection, 0, 64)
	cur := ReaderRenderSection{ID: "docx-sec-1", Title: model.Title}
	inP := false
	heading := false
	var textBuf strings.Builder
	secN := 1
	flushParagraph := func() {
		t := cleanText(textBuf.String())
		if t == "" {
			return
		}
		if heading {
			if len(cur.Paragraphs) > 0 || cur.Title != "" {
				sections = append(sections, cur)
				secN++
			}
			cur = ReaderRenderSection{
				ID:    fmt.Sprintf("docx-sec-%d", secN),
				Title: t,
				Anchors: []ReaderRenderAnchor{{
					ID:             fmt.Sprintf("docx-sec-%d-anchor", secN),
					Label:          t,
					ParagraphIndex: 0,
					Kind:           "heading",
				}},
			}
		} else {
			cur.Paragraphs = append(cur.Paragraphs, t)
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if name == "p" {
				inP = true
				heading = false
				textBuf.Reset()
			}
			if inP && name == "pstyle" {
				for _, a := range t.Attr {
					if strings.ToLower(a.Name.Local) == "val" {
						v := strings.ToLower(a.Value)
						if strings.HasPrefix(v, "heading") || strings.HasPrefix(v, "заголовок") {
							heading = true
						}
					}
				}
			}
		case xml.EndElement:
			if strings.ToLower(t.Name.Local) == "p" {
				flushParagraph()
				inP = false
			}
		case xml.CharData:
			if inP {
				textBuf.WriteString(string(t))
			}
		}
	}
	if len(cur.Paragraphs) > 0 || cur.Title != "" {
		sections = append(sections, cur)
	}
	model.Sections = sections
	enrichDocxRefsAndFootnotes(docXML, model.Sections, footnoteMap)
	return nil
}

func fillEpubModel(raw []byte, model *ReaderRenderModel) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	rootFile, err := epubpkg.RootfilePath(zr)
	if err != nil {
		return err
	}
	pkg, err := epubpkg.ParseOPF(zr, rootFile)
	if err != nil {
		return err
	}
	if pkg.Title != "" {
		model.Title = pkg.Title
	}
	if pkg.Language != "" {
		model.Language = pkg.Language
	}
	if coverPath := pkg.CoverZipPath(); coverPath != "" {
		if b, ct := readZipFileWithType(zr, coverPath); len(b) > 0 {
			model.CoverData = fmt.Sprintf("data:%s;base64,%s", ct, base64.StdEncoding.EncodeToString(b))
		}
	}
	sections := make([]ReaderRenderSection, 0, len(pkg.Spine))
	tocItems := make([]ReaderTOCItem, 0, len(pkg.Spine))
	for i, sr := range pkg.Spine {
		it, ok := pkg.Manifest[sr.IDRef]
		if !ok || it.Href == "" {
			continue
		}
		href := it.Href
		xPath := epubpkg.JoinPath(pkg.OPFDir, href)
		b, _ := readZipFileWithType(zr, xPath)
		if len(b) == 0 {
			continue
		}
		secID := fmt.Sprintf("epub-sec-%d", i+1)
		title, pars, footnotes, refs := extractXhtmlText(b, secID)
		if len(pars) == 0 && len(footnotes) == 0 {
			continue
		}
		secTitle := firstNonEmptyClean(title, path.Base(href))
		if strings.TrimSpace(secTitle) != "" {
			tocItems = append(tocItems, ReaderTOCItem{Label: cleanText(secTitle), Href: "#" + secID})
		}
		sections = append(sections, ReaderRenderSection{
			ID:        secID,
			Title:     secTitle,
			Paragraphs: pars,
			Footnotes: footnoteTexts(footnotes),
			Refs: refs,
			FootnoteItems: footnotes,
			Anchors: []ReaderRenderAnchor{{
				ID:             secID + "-anchor",
				Label:          secTitle,
				ParagraphIndex: 0,
				Kind:           "section",
			}},
		})
	}
	model.Sections = sections
	if len(tocItems) > 0 {
		model.TOC = tocItems
	}
	return nil
}

func extractXhtmlText(raw []byte, secID string) (string, []string, []ReaderRenderFootnote, []ReaderRenderRefLink) {
	s := string(raw)
	title := ""
	if m := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindStringSubmatch(s); len(m) > 1 {
		title = stripTags(m[1])
	}
	// Сначала собираем сноски и множество известных id — затем ссылки href="#...".
	noteMatches := regexp.MustCompile(`(?is)<(aside|li)([^>]*)>(.*?)</(aside|li)>`).FindAllStringSubmatch(s, -1)
	notes := make([]ReaderRenderFootnote, 0, len(noteMatches))
	footIDSet := map[string]struct{}{}
	nIDRe := regexp.MustCompile(`(?is)\sid=["']([^"']+)["']`)
	addFootKeys := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		footIDSet[id] = struct{}{}
		if i := strings.Index(id, ":"); i >= 0 {
			footIDSet[id[i+1:]] = struct{}{}
		}
	}
	for _, m := range noteMatches {
		attrs := strings.ToLower(m[2])
		if !strings.Contains(attrs, "footnote") && !strings.Contains(attrs, "note") && !strings.Contains(attrs, "endnote") {
			continue
		}
		if t := cleanText(stripTags(m[3])); t != "" {
			noteID := ""
			if idm := nIDRe.FindStringSubmatch(m[2]); len(idm) > 1 {
				noteID = normalizeAnchorID(secID, idm[1])
				addFootKeys(noteID)
				addFootKeys(idm[1])
			}
			if noteID == "" {
				noteID = fmt.Sprintf("%s-note-%d", secID, len(notes)+1)
			}
			addFootKeys(noteID)
			notes = append(notes, ReaderRenderFootnote{ID: noteID, Text: t})
		}
	}
	bodyForParas := s
	for _, m := range noteMatches {
		attrs := strings.ToLower(m[2])
		if !strings.Contains(attrs, "footnote") && !strings.Contains(attrs, "note") && !strings.Contains(attrs, "endnote") {
			continue
		}
		bodyForParas = strings.Replace(bodyForParas, m[0], " ", 1)
	}
	pMatches := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`).FindAllStringSubmatch(bodyForParas, -1)
	pars := make([]string, 0, len(pMatches))
	refs := make([]ReaderRenderRefLink, 0, len(pMatches))
	refRe := regexp.MustCompile(`(?is)<a([^>]*)href="#([^"]+)"([^>]*)>(.*?)</a>`)
	for _, m := range pMatches {
		pIdx := len(pars)
		if t := cleanText(stripTags(m[1])); t != "" {
			pars = append(pars, t)
			refMatches := refRe.FindAllStringSubmatch(m[1], -1)
			for i, rm := range refMatches {
				noteFrag := cleanText(rm[2])
				if noteFrag == "" {
					continue
				}
				attrs := strings.ToLower(rm[1] + " " + rm[3])
				canon := normalizeAnchorID(secID, noteFrag)
				_, okCanon := footIDSet[canon]
				_, okFrag := footIDSet[noteFrag]
				_, okPref := footIDSet[secID+":"+noteFrag]
				isNoteref := strings.Contains(attrs, "noteref") || strings.Contains(attrs, "doc-noteref") ||
					strings.Contains(attrs, "role=\"doc-noteref\"") || strings.Contains(attrs, "role='doc-noteref'") ||
					(strings.Contains(attrs, "epub:type") && strings.Contains(attrs, "noteref"))
				isLikely := strings.Contains(strings.ToLower(noteFrag), "note") || strings.Contains(strings.ToLower(noteFrag), "fn") ||
					strings.Contains(attrs, "footnote")
				if !(okCanon || okFrag || okPref || isNoteref || isLikely) {
					continue
				}
				refs = append(refs, ReaderRenderRefLink{
					ID:             fmt.Sprintf("%s-ref-%d-%d", secID, pIdx+1, i+1),
					NoteID:         canon,
					Label:          cleanText(stripTags(rm[4])),
					ParagraphIndex: pIdx,
				})
			}
		}
	}
	linkFootnotesWithRefs(notes, refs)
	return title, pars, notes, refs
}

func footnoteTexts(items []ReaderRenderFootnote) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if t := cleanText(it.Text); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func normalizeAnchorID(secID, raw string) string {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "#"))
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, ":") {
		return raw
	}
	return secID + ":" + raw
}

func linkFootnotesWithRefs(footnotes []ReaderRenderFootnote, refs []ReaderRenderRefLink) {
	noteIndex := make(map[string]int, len(footnotes))
	for i := range footnotes {
		noteIndex[footnotes[i].ID] = i
	}
	for _, r := range refs {
		idx, ok := noteIndex[r.NoteID]
		if !ok {
			continue
		}
		if footnotes[idx].BackRefID == "" {
			footnotes[idx].BackRefID = r.ID
		}
	}
}

func enrichFB2RefsAndFootnotes(raw []byte, sections *[]ReaderRenderSection) {
	if len(*sections) == 0 || len(raw) == 0 {
		return
	}
	src := string(raw)
	noteBody := regexp.MustCompile(`(?is)<body[^>]*name=["']notes["'][^>]*>(.*?)</body>`).FindStringSubmatch(src)
	noteMap := map[string]string{}
	if len(noteBody) > 1 {
		secRe := regexp.MustCompile(`(?is)<section[^>]*id=["']([^"']+)["'][^>]*>(.*?)</section>`)
		for _, sm := range secRe.FindAllStringSubmatch(noteBody[1], -1) {
			noteMap[sm[1]] = cleanText(stripTags(sm[2]))
		}
	}
	refRe := regexp.MustCompile(`(?is)<a[^>]*l:href=["']#([^"']+)["'][^>]*>(.*?)</a>`)
	for si := range *sections {
		sec := &(*sections)[si]
		pCounter := 0
		for pIdx, pText := range sec.Paragraphs {
			matches := refRe.FindAllStringSubmatch(pText, -1)
			if len(matches) == 0 {
				continue
			}
			for _, m := range matches {
				noteRaw := cleanText(m[1])
				if noteRaw == "" {
					continue
				}
				noteID := normalizeAnchorID(sec.ID, noteRaw)
				refID := fmt.Sprintf("%s-ref-%d-%d", sec.ID, pIdx+1, pCounter+1)
				pCounter++
				sec.Refs = append(sec.Refs, ReaderRenderRefLink{
					ID:             refID,
					NoteID:         noteID,
					Label:          cleanText(stripTags(m[2])),
					ParagraphIndex: pIdx,
				})
				if noteText := noteMap[noteRaw]; noteText != "" {
					sec.FootnoteItems = append(sec.FootnoteItems, ReaderRenderFootnote{
						ID:        noteID,
						Text:      noteText,
						BackRefID: refID,
					})
				}
			}
		}
	}
}

func parseDocxFootnotesMap(zr *zip.Reader) map[string]string {
	raw, _ := readZipFileWithType(zr, "word/footnotes.xml")
	if len(raw) == 0 {
		return nil
	}
	re := regexp.MustCompile(`(?is)<w:footnote[^>]*w:id="(-?\d+)"[^>]*>(.*?)</w:footnote>`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		id := strings.TrimSpace(m[1])
		if id == "" || strings.HasPrefix(id, "-") {
			continue
		}
		txt := cleanText(stripTags(m[2]))
		if txt != "" {
			out[id] = txt
		}
	}
	return out
}

func enrichDocxRefsAndFootnotes(docXML []byte, sections []ReaderRenderSection, footnoteMap map[string]string) {
	if len(docXML) == 0 || len(sections) == 0 || len(footnoteMap) == 0 {
		return
	}
	pRe := regexp.MustCompile(`(?is)<w:p[^>]*>(.*?)</w:p>`)
	refRe := regexp.MustCompile(`(?is)<w:footnoteReference[^>]*w:id="(\d+)"`)
	sectionIdx := 0
	paragraphIdx := 0
	for _, pm := range pRe.FindAllStringSubmatch(string(docXML), -1) {
		chunk := pm[1]
		if strings.Contains(strings.ToLower(chunk), "w:pstyle") && strings.Contains(strings.ToLower(chunk), "heading") {
			continue
		}
		text := cleanText(stripTags(chunk))
		if text == "" {
			continue
		}
		for sectionIdx < len(sections) && paragraphIdx >= len(sections[sectionIdx].Paragraphs) {
			sectionIdx++
			paragraphIdx = 0
		}
		if sectionIdx >= len(sections) {
			return
		}
		sec := &sections[sectionIdx]
		matches := refRe.FindAllStringSubmatch(chunk, -1)
		for i, rm := range matches {
			fid := strings.TrimSpace(rm[1])
			noteText := footnoteMap[fid]
			if noteText == "" {
				continue
			}
			noteID := fmt.Sprintf("%s:footnote-%s", sec.ID, fid)
			refID := fmt.Sprintf("%s-ref-%d-%d", sec.ID, paragraphIdx+1, i+1)
			sec.Refs = append(sec.Refs, ReaderRenderRefLink{
				ID:             refID,
				NoteID:         noteID,
				Label:          fid,
				ParagraphIndex: paragraphIdx,
			})
			sec.FootnoteItems = append(sec.FootnoteItems, ReaderRenderFootnote{
				ID:        noteID,
				Text:      noteText,
				BackRefID: refID,
			})
		}
		paragraphIdx++
	}
}

func parseFB2Structure(raw []byte) ([]ReaderRenderSection, []ReaderTOCItem) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.CharsetReader = makeCharsetReader

	type secBuilder struct {
		id       string
		title    string
		paras    []string
		refs     []ReaderRenderRefLink
		notes    []ReaderRenderFootnote
		refCount map[int]int
	}

	var bodyName string
	var inSection bool
	var inTitle bool
	var inP bool
	var inA bool
	var currentHref string
	var pText strings.Builder
	var aText strings.Builder
	var titleText strings.Builder

	notesByID := map[string]string{}
	stack := make([]*secBuilder, 0, 16)
	sections := make([]ReaderRenderSection, 0, 128)
	toc := make([]ReaderTOCItem, 0, 128)

	flushParagraph := func() {
		if !inP || len(stack) == 0 {
			return
		}
		txt := cleanText(pText.String())
		pText.Reset()
		inP = false
		if txt == "" {
			return
		}
		sb := stack[len(stack)-1]
		sb.paras = append(sb.paras, txt)
	}

	flushTitle := func() {
		if len(stack) == 0 {
			return
		}
		txt := cleanText(titleText.String())
		titleText.Reset()
		if txt == "" {
			return
		}
		sb := stack[len(stack)-1]
		if sb.title == "" {
			sb.title = txt
		}
	}

	closeSection := func() {
		if len(stack) == 0 {
			return
		}
		sb := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if bodyName == "notes" {
			id := strings.TrimPrefix(sb.id, "fb2-note-")
			if id == "" {
				id = sb.id
			}
			if strings.TrimSpace(sb.title) != "" || len(sb.paras) > 0 {
				txt := cleanText(strings.Join(sb.paras, " "))
				if txt == "" && sb.title != "" {
					txt = sb.title
				}
				if txt != "" {
					notesByID[id] = txt
				}
			}
			return
		}

		if len(sb.paras) == 0 && strings.TrimSpace(sb.title) == "" {
			return
		}

		secID := sb.id
		if secID == "" {
			secID = fmt.Sprintf("fb2-sec-%d", len(sections)+1)
		}
		title := sb.title
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("Глава %d", len(sections)+1)
		}

		sec := ReaderRenderSection{
			ID:         secID,
			Title:      title,
			Paragraphs: sb.paras,
			Refs:       sb.refs,
			FootnoteItems: sb.notes,
			Anchors: []ReaderRenderAnchor{{
				ID:             secID + "-anchor",
				Label:          title,
				ParagraphIndex: 0,
				Kind:           "section",
			}},
		}
		sections = append(sections, sec)
		toc = append(toc, ReaderTOCItem{Label: cleanText(title), Href: "#" + secID})
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "body":
				bodyName = ""
				for _, a := range t.Attr {
					if strings.ToLower(a.Name.Local) == "name" {
						bodyName = strings.ToLower(strings.TrimSpace(a.Value))
					}
				}
			case "section":
				inSection = true
				secID := ""
				for _, a := range t.Attr {
					if strings.ToLower(a.Name.Local) == "id" {
						secID = strings.TrimSpace(a.Value)
					}
				}
				if bodyName == "notes" {
					if secID != "" {
						secID = "fb2-note-" + secID
					} else {
						secID = fmt.Sprintf("fb2-note-%d", len(notesByID)+1)
					}
				} else if secID == "" {
					secID = fmt.Sprintf("fb2-sec-%d", len(sections)+len(stack)+1)
				}
				stack = append(stack, &secBuilder{id: secID, paras: make([]string, 0, 32), refCount: map[int]int{}})
			case "title":
				inTitle = true
				titleText.Reset()
			case "p":
				inP = true
				pText.Reset()
			case "a":
				if !inP || len(stack) == 0 || bodyName == "notes" {
					break
				}
				inA = true
				currentHref = ""
				aText.Reset()
				for _, a := range t.Attr {
					ln := strings.ToLower(a.Name.Local)
					if ln == "href" || strings.HasSuffix(ln, "href") {
						currentHref = strings.TrimSpace(strings.TrimPrefix(a.Value, "#"))
					}
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			switch name {
			case "a":
				if inA && currentHref != "" && len(stack) > 0 {
					sb := stack[len(stack)-1]
					pIdx := len(sb.paras)
					sb.refCount[pIdx]++
					refID := fmt.Sprintf("%s-ref-%d-%d", sb.id, pIdx+1, sb.refCount[pIdx])
					noteID := normalizeAnchorID(sb.id, currentHref)
					lbl := cleanText(aText.String())
					if lbl == "" {
						lbl = "↗"
					}
					sb.refs = append(sb.refs, ReaderRenderRefLink{
						ID:             refID,
						NoteID:         noteID,
						Label:          lbl,
						ParagraphIndex: pIdx,
					})
					pText.WriteString(" ")
					pText.WriteString(lbl)
					pText.WriteString(" ")
					inA = false
					currentHref = ""
					aText.Reset()
				}
			case "p":
				if inTitle {
					flushTitle()
					inP = false
					pText.Reset()
				} else {
					flushParagraph()
				}
			case "title":
				inTitle = false
				flushTitle()
			case "section":
				flushParagraph()
				inSection = false
				closeSection()
			case "body":
				bodyName = ""
			}
		case xml.CharData:
			if inTitle {
				titleText.WriteString(string(t))
			} else if inA {
				aText.WriteString(string(t))
			} else if inP && inSection {
				pText.WriteString(string(t))
			}
		}
	}

	// Link refs -> notes by id (same section scope).
	for si := range sections {
		sec := &sections[si]
		for _, r := range sec.Refs {
			rawID := strings.TrimPrefix(r.NoteID, sec.ID+":")
			if txt := notesByID[rawID]; txt != "" {
				sec.FootnoteItems = append(sec.FootnoteItems, ReaderRenderFootnote{ID: r.NoteID, Text: txt, BackRefID: r.ID})
			}
		}
	}

	if len(toc) == 0 {
		return sections, nil
	}
	return sections, toc
}

func parseViewportProfile(r *http.Request) ReaderViewportProfile {
	q := r.URL.Query()
	vw, _ := strconv.Atoi(q.Get("vw"))
	vh, _ := strconv.Atoi(q.Get("vh"))
	mg, _ := strconv.Atoi(q.Get("mg"))
	orientation := strings.TrimSpace(strings.ToLower(q.Get("or")))
	mode := strings.TrimSpace(strings.ToLower(q.Get("mode")))
	fs, _ := strconv.ParseFloat(q.Get("fs"), 64)
	lh, _ := strconv.ParseFloat(q.Get("lh"), 64)
	if vw < 320 {
		vw = 1200
	}
	if vh < 300 {
		vh = 900
	}
	if mg < 0 {
		mg = 20
	}
	if fs <= 0 {
		fs = 20
	}
	if lh <= 0 {
		lh = 1.7
	}
	if orientation != "landscape" {
		orientation = "portrait"
	}
	switch mode {
	case "paged", "double-page", "scroll":
	default:
		mode = "paged"
	}
	return ReaderViewportProfile{Width: vw, Height: vh, FontSize: fs, LineHeight: lh, Margin: mg, Orientation: orientation, Mode: mode}
}

func paginateRenderSections(sections []ReaderRenderSection, vp ReaderViewportProfile) []ReaderRenderPage {
	if len(sections) == 0 {
		return nil
	}
	// Conservative viewport model: we must ensure a page fits without vertical scroll.
	// Double-page mode reduces available width per page roughly by half.
	effW := max(240, vp.Width-vp.Margin*2)
	if vp.Mode == "double-page" {
		gap := 24
		effW = max(200, (effW-gap)/2)
	}
	// Reserve chrome + status bar space.
	effH := max(260, vp.Height-180)
	charsPerLine := int(float64(effW) / (vp.FontSize * 0.56))
	linesPerPage := int(float64(effH) / (vp.FontSize * vp.LineHeight))
	if vp.Mode == "scroll" {
		linesPerPage = max(linesPerPage, 36)
	}
	if vp.Orientation == "landscape" {
		charsPerLine = int(float64(charsPerLine) * 1.15)
	}
	if charsPerLine < 24 {
		charsPerLine = 24
	}
	if linesPerPage < 10 {
		linesPerPage = 10
	}
	// Safety factor to avoid overflow due to headings, paragraph margins, long words, and footnotes.
	budget := int(float64(charsPerLine*linesPerPage) * 0.82)
	if budget < 600 {
		budget = 600
	}

	pages := make([]ReaderRenderPage, 0, 256)
	for _, sec := range sections {
		if len(sec.Paragraphs) == 0 {
			continue
		}
		start := 0
		acc := 0
		for i, p := range sec.Paragraphs {
			acc += len([]rune(p)) + 16
			if acc >= budget {
				pages = append(pages, ReaderRenderPage{
					SectionID:       sec.ID,
					ParagraphFrom:   start,
					ParagraphTo:     i,
					Index:           len(pages) + 1,
					PrimaryAnchorID: sectionAnchorForParagraph(sec, start),
				})
				start = i + 1
				acc = 0
			}
		}
		if start < len(sec.Paragraphs) {
			pages = append(pages, ReaderRenderPage{
				SectionID:       sec.ID,
				ParagraphFrom:   start,
				ParagraphTo:     len(sec.Paragraphs) - 1,
				Index:           len(pages) + 1,
				PrimaryAnchorID: sectionAnchorForParagraph(sec, start),
			})
		}
	}
	total := len(pages)
	for i := range pages {
		if total > 1 {
			pages[i].Fraction = float64(i) / float64(total-1)
		}
	}
	return pages
}

func sectionAnchorForParagraph(sec ReaderRenderSection, pIdx int) string {
	bestID := ""
	bestPos := -1
	for _, a := range sec.Anchors {
		if a.ParagraphIndex <= pIdx && a.ParagraphIndex >= bestPos {
			bestID = a.ID
			bestPos = a.ParagraphIndex
		}
	}
	return bestID
}

func tocFromSections(sections []ReaderRenderSection) []ReaderTOCItem {
	out := make([]ReaderTOCItem, 0, len(sections))
	for _, s := range sections {
		if strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.ID) == "" {
			continue
		}
		out = append(out, ReaderTOCItem{
			Label: cleanText(s.Title),
			Href:  "#" + s.ID,
		})
	}
	return out
}

func readZipFileWithType(zr *zip.Reader, name string) ([]byte, string) {
	b, err := epubpkg.ReadFile(zr, name)
	if err != nil {
		return nil, ""
	}
	ct := "application/octet-stream"
	if zf := epubpkg.FindFile(zr, name); zf != nil {
		ct = epubpkg.MimeForExt(path.Ext(zf.Name))
	}
	return b, ct
}
