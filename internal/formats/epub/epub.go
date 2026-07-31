package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var errCoverNotFound = errors.New("cover not found")

// ManifestItem — элемент manifest OPF.
type ManifestItem struct {
	ID         string
	Href       string
	MediaType  string
	Properties string
}

// SpineRef — ссылка spine на manifest.
type SpineRef struct {
	IDRef    string
	LinearNo bool
}

// Package — разобранный OPF EPUB.
type Package struct {
	OPFPath   string
	OPFDir    string
	Manifest  map[string]ManifestItem
	Spine     []SpineRef
	CoverID   string
	Title     string
	Language  string
}

// NewReader открывает EPUB из байтов в памяти.
func NewReader(content []byte) (*zip.Reader, error) {
	return zip.NewReader(bytes.NewReader(content), int64(len(content)))
}

// JoinPath склеивает путь OPF и href (всегда прямые слэши внутри ZIP).
func JoinPath(dir, href string) string {
	href = strings.TrimPrefix(href, "/")
	if dir == "" {
		return href
	}
	dir = strings.TrimSuffix(dir, "/")
	return dir + "/" + href
}

// FindFile ищет файл в ZIP без учёта регистра и слэшей.
func FindFile(zr *zip.Reader, name string) *zip.File {
	name = strings.TrimPrefix(name, "/")
	candidates := []string{name, strings.ReplaceAll(name, "\\", "/")}
	for _, f := range zr.File {
		fn := strings.TrimPrefix(f.Name, "/")
		for _, c := range candidates {
			if strings.EqualFold(fn, c) {
				return f
			}
		}
	}
	return nil
}

// ReadFile читает файл из ZIP по имени.
func ReadFile(zr *zip.Reader, name string) ([]byte, error) {
	zf := FindFile(zr, name)
	if zf == nil {
		return nil, fmt.Errorf("epub: файл не найден: %s", name)
	}
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// MimeForExt возвращает MIME для расширения изображения.
func MimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// RootfilePath возвращает путь к OPF из container.xml.
func RootfilePath(zr *zip.Reader) (string, error) {
	raw, err := ReadFile(zr, "META-INF/container.xml")
	if err != nil {
		return "", err
	}
	var c containerXML
	if err := xml.Unmarshal(raw, &c); err != nil {
		return "", err
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return "", errors.New("epub: rootfile not found")
	}
	return c.Rootfiles[0].FullPath, nil
}

type opfXML struct {
	Metadata struct {
		Title string `xml:"title"`
		Lang  string `xml:"language"`
		Meta  []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Itemrefs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func cleanMeta(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

// ParseOPF читает и разбирает OPF по пути внутри ZIP.
func ParseOPF(zr *zip.Reader, opfPath string) (*Package, error) {
	raw, err := ReadFile(zr, opfPath)
	if err != nil {
		return nil, err
	}
	var p opfXML
	if err := xml.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	pkg := &Package{
		OPFPath:  opfPath,
		Manifest: make(map[string]ManifestItem, len(p.Manifest.Items)),
	}
	if i := strings.LastIndex(opfPath, "/"); i >= 0 {
		pkg.OPFDir = opfPath[:i]
	}
	coverID := ""
	for _, it := range p.Manifest.Items {
		pkg.Manifest[it.ID] = ManifestItem{
			ID:         it.ID,
			Href:       it.Href,
			MediaType:  it.MediaType,
			Properties: it.Properties,
		}
		if strings.Contains(strings.ToLower(it.Properties), "cover-image") {
			coverID = it.ID
		}
	}
	if coverID == "" {
		for _, m := range p.Metadata.Meta {
			if strings.EqualFold(m.Name, "cover") {
				coverID = strings.TrimSpace(m.Content)
				break
			}
		}
	}
	if coverID == "" {
		for id, it := range pkg.Manifest {
			if strings.HasPrefix(strings.ToLower(it.MediaType), "image/") &&
				(strings.Contains(strings.ToLower(id), "cover") ||
					strings.Contains(strings.ToLower(it.Properties), "cover-image")) {
				coverID = id
				break
			}
		}
	}
	pkg.CoverID = coverID
	pkg.Title = cleanMeta(p.Metadata.Title)
	pkg.Language = cleanMeta(p.Metadata.Lang)
	for _, it := range p.Spine.Itemrefs {
		id := strings.TrimSpace(it.IDRef)
		if id == "" {
			continue
		}
		linearNo := strings.EqualFold(strings.TrimSpace(it.Linear), "no")
		pkg.Spine = append(pkg.Spine, SpineRef{IDRef: id, LinearNo: linearNo})
	}
	return pkg, nil
}

// OpenPackage открывает EPUB и парсит OPF.
func OpenPackage(content []byte) (*zip.Reader, *Package, error) {
	zr, err := NewReader(content)
	if err != nil {
		return nil, nil, err
	}
	opfPath, err := RootfilePath(zr)
	if err != nil {
		return nil, nil, err
	}
	pkg, err := ParseOPF(zr, opfPath)
	if err != nil {
		return nil, nil, err
	}
	return zr, pkg, nil
}

func itemHasNav(it ManifestItem) bool {
	for _, t := range strings.Fields(strings.ToLower(it.Properties)) {
		if t == "nav" {
			return true
		}
	}
	return false
}

// IsHTMLishMedia сообщает, подходит ли manifest-элемент для извлечения текста.
func IsHTMLishMedia(mt, href string) bool {
	mt = strings.ToLower(mt)
	h := strings.ToLower(href)
	if strings.Contains(mt, "svg") || strings.HasSuffix(h, ".svg") {
		return false
	}
	if strings.Contains(mt, "javascript") || strings.HasSuffix(h, ".js") {
		return false
	}
	if strings.HasPrefix(mt, "image/") {
		return false
	}
	if strings.Contains(mt, "html") {
		return true
	}
	if strings.Contains(mt, "xml") {
		return true
	}
	return strings.HasSuffix(h, ".xhtml") || strings.HasSuffix(h, ".html") || strings.HasSuffix(h, ".htm")
}

// HTMLSpinePaths возвращает пути к HTML/XHTML главам по порядку spine.
func (pkg *Package) HTMLSpinePaths() []string {
	itemByID := pkg.Manifest
	var hrefs []string
	add := func(it ManifestItem) {
		if itemHasNav(it) || !IsHTMLishMedia(it.MediaType, it.Href) {
			return
		}
		hrefs = append(hrefs, JoinPath(pkg.OPFDir, it.Href))
	}
	if len(pkg.Spine) > 0 {
		for _, sr := range pkg.Spine {
			if sr.LinearNo {
				continue
			}
			if it, ok := itemByID[sr.IDRef]; ok {
				add(it)
			}
		}
	}
	if len(hrefs) == 0 {
		for _, sr := range pkg.Spine {
			if it, ok := itemByID[sr.IDRef]; ok {
				add(it)
			}
		}
	}
	if len(hrefs) == 0 {
		for _, it := range itemByID {
			add(it)
		}
	}
	return hrefs
}

// ManifestHref возвращает href manifest-элемента по id.
func (pkg *Package) ManifestHref(id string) string {
	if it, ok := pkg.Manifest[id]; ok {
		return it.Href
	}
	return ""
}

// CoverZipPath возвращает путь к файлу обложки внутри ZIP.
func (pkg *Package) CoverZipPath() string {
	if pkg.CoverID == "" {
		return ""
	}
	href := pkg.ManifestHref(pkg.CoverID)
	if href == "" {
		return ""
	}
	return JoinPath(pkg.OPFDir, href)
}

// ExtractCover извлекает байты обложки и MIME из EPUB.
func ExtractCover(content []byte) ([]byte, string, error) {
	zr, pkg, err := OpenPackage(content)
	if err != nil {
		return nil, "", err
	}
	coverPath := pkg.CoverZipPath()
	if coverPath == "" {
		return nil, "", errCoverNotFound
	}
	data, err := readZipEntry(zr, coverPath)
	if err != nil {
		coverBase := path.Base(coverPath)
		for _, f := range zr.File {
			if path.Base(f.Name) == coverBase {
				data, err = readZipEntryFile(f)
				if err == nil {
					return data, mimeForManifest(pkg, pkg.CoverID), nil
				}
			}
		}
		return nil, "", errCoverNotFound
	}
	return data, mimeForManifest(pkg, pkg.CoverID), nil
}

func mimeForManifest(pkg *Package, id string) string {
	if it, ok := pkg.Manifest[id]; ok && it.MediaType != "" {
		return it.MediaType
	}
	return MimeForExt(path.Ext(pkg.CoverZipPath()))
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	zf := FindFile(zr, name)
	if zf == nil {
		return nil, fmt.Errorf("epub: %s not found", name)
	}
	return readZipEntryFile(zf)
}

func readZipEntryFile(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// ErrCoverNotFound — обложка не найдена в EPUB.
func ErrCoverNotFound() error {
	return errCoverNotFound
}
