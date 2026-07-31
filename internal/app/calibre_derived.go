// Фрагменты логики, вдохновлённые Calibre 9.x (GPL-3.0), переписанные на Go без Python-рантайма.
// Исходники-ориентиры в репозитории:
//   - calibre-9.8.0/src/calibre/ebooks/compression/tcr.py  (функция decompress)
//   - calibre-9.8.0/format_converter_extracted/.../htmlz_input.py (ZIP + index.html)
//   - ODF: content.xml внутри ODT (ZIP), аналогично плагину odt_input (без полного Extract).
//   - RB, PML/PMLZ — см. calibre_derived_rb_pml.go (reader.py, strip_pml).
//
// CHM, LIT, LRF, полный PML_HTMLizer и др. — объёмно; по мере необходимости портируются отдельно.

package app
import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	stdhtml "html"
)

// tcrDecompress распаковывает eReader / Psion TCR («!!8-Bit!!» + словарь 256 записей + коды).
// Поведение соответствует calibre.ebooks.compression.tcr.decompress.
func tcrDecompress(raw []byte) ([]byte, error) {
	if len(raw) < 9 || string(raw[:9]) != "!!8-Bit!!" {
		return nil, errors.New("tcr: неверный заголовок (ожидается !!8-Bit!!)")
	}
	pos := 9
	entries := make([][]byte, 256)
	for i := 0; i < 256; i++ {
		if pos >= len(raw) {
			return nil, errors.New("tcr: обрезан словарь кодов")
		}
		elen := int(raw[pos])
		pos++
		if elen < 0 || pos+elen > len(raw) {
			return nil, errors.New("tcr: обрезана запись словаря")
		}
		entries[i] = append([]byte(nil), raw[pos:pos+elen]...)
		pos += elen
	}
	var out bytes.Buffer
	for pos < len(raw) {
		idx := int(raw[pos])
		pos++
		if idx < 0 || idx > 255 {
			return nil, fmt.Errorf("tcr: недопустимый код %d", idx)
		}
		_, _ = out.Write(entries[idx])
	}
	return out.Bytes(), nil
}

func tcrToParagraphs(raw []byte) ([]string, error) {
	txt, err := tcrDecompress(raw)
	if err != nil {
		return nil, err
	}
	s, _ := decodeBytesToUTF8(txt, "txt")
	return paragraphsFromPlainText(string(s)), nil
}

var reODTParagraph = regexp.MustCompile(`(?i)<text:p[^>]*>([\s\S]*?)</text:p>`)
var reXMLTags = regexp.MustCompile(`<[^>]+>`)

func stripTagsAndDecode(s string) string {
	s = reXMLTags.ReplaceAllString(s, " ")
	return strings.TrimSpace(stdhtml.UnescapeString(s))
}

// odtToParagraphs извлекает текст из OpenDocument (ODT): ZIP + content.xml, блоки text:p.
func odtToParagraphs(raw []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("odt: не zip: %w", err)
	}
	var content []byte
	for _, f := range zr.File {
		if f.Name == "content.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			content, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if len(content) == 0 {
		return nil, errors.New("odt: нет content.xml")
	}
	body := string(content)
	var paras []string
	for _, sm := range reODTParagraph.FindAllStringSubmatch(body, -1) {
		if len(sm) < 2 {
			continue
		}
		t := stripTagsAndDecode(sm[1])
		if t != "" {
			paras = append(paras, t)
		}
	}
	if len(paras) == 0 {
		return nil, errors.New("odt: не найдено текста в text:p")
	}
	return paras, nil
}

func zipTopLevelHTMLName(zr *zip.Reader) (string, error) {
	priority := []string{"index.html", "index.xhtml", "index.htm"}
	var candidates []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		n := f.Name
		if strings.Contains(n, "..") {
			continue
		}
		// Только файлы в корне архива (без «папок»), как в Calibre htmlz_input.
		if strings.Contains(n, "/") || strings.Contains(n, "\\") {
			continue
		}
		ext := strings.ToLower(path.Ext(n))
		if ext == ".html" || ext == ".htm" || ext == ".xhtml" {
			candidates = append(candidates, n)
		}
	}
	for _, p := range priority {
		for _, c := range candidates {
			if strings.EqualFold(c, p) {
				return c, nil
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return "", errors.New("htmlz: нет html на верхнем уровне архива")
}

// htmlzToParagraphs — HTMLZ: zip с одним главным HTML на верхнем уровне (как Calibre htmlz_input).
func htmlzToParagraphs(raw []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("htmlz: не zip: %w", err)
	}
	name, err := zipTopLevelHTMLName(zr)
	if err != nil {
		return nil, err
	}
	var htmlRaw []byte
	for _, f := range zr.File {
		if f.Name != name || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		htmlRaw, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		break
	}
	if len(htmlRaw) == 0 {
		return nil, errors.New("htmlz: пустой html")
	}
	s, _ := decodeBytesToUTF8(htmlRaw, "html")
	return htmlToParagraphs(string(s))
}
