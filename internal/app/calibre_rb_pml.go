// Порции логики Calibre 9.x (GPL-3.0) → Go без Python.
// Ориентиры:
//   - calibre-9.8.0/src/calibre/ebooks/rb/__init__.py (HEADER), reader.py (разбор TOC, zlib-страницы)
//   - calibre-9.8.0/src/calibre/ebooks/pml/pmlconverter.py (strip_pml — вычищание разметки до плоского текста)
//
// PML: здесь не полный PML_HTMLizer (сотни строк состояний), а strip_pml + абзацы — читаемый текст для каталога/FB2.

package app
import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// HEADER Rocket eBook (NUVO) — см. calibre.ebooks.rb.HEADER.
var rbRocketHeader = []byte{0xb0, 0x0c, 0xb0, 0x0c, 0x02, 0x00, 'N', 'U', 'V', 'O', 0, 0, 0, 0}

type rbTOCItem struct {
	Name   string
	Size   uint32
	Offset uint32
	Flags  uint32
}

func rbReadTOC(data []byte) ([]rbTOCItem, error) {
	if len(data) < 32 {
		return nil, errors.New("rb: файл слишком короткий")
	}
	if !bytes.Equal(data[:14], rbRocketHeader) {
		return nil, errors.New("rb: нет заголовка Rocket eBook (NUVO)")
	}
	decl := binary.LittleEndian.Uint32(data[28:32])
	if uint64(decl) != uint64(len(data)) {
		return nil, fmt.Errorf("rb: размер в заголовке (%d) ≠ размеру файла (%d)", decl, len(data))
	}
	tocOff := int(binary.LittleEndian.Uint32(data[24:28]))
	if tocOff < 0 || tocOff+4 > len(data) {
		return nil, errors.New("rb: некорректное смещение TOC")
	}
	n := int(binary.LittleEndian.Uint32(data[tocOff : tocOff+4]))
	if n < 0 || n > 100000 {
		return nil, fmt.Errorf("rb: подозрительное число страниц: %d", n)
	}
	pos := tocOff + 4
	entryBytes := 32 + 4 + 4 + 4
	if pos+n*entryBytes > len(data) {
		return nil, errors.New("rb: TOC выходит за пределы файла")
	}
	out := make([]rbTOCItem, 0, n)
	for i := 0; i < n; i++ {
		if pos+32 > len(data) {
			break
		}
		nameBuf := data[pos : pos+32]
		pos += 32
		sz := binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		off := binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		fl := binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		name := rbDecodeTOCName(nameBuf)
		out = append(out, rbTOCItem{Name: name, Size: sz, Offset: off, Flags: fl})
	}
	return out, nil
}

func rbDecodeTOCName(b []byte) string {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	raw := string(b)
	if u, err := url.PathUnescape(raw); err == nil {
		raw = u
	}
	return raw
}

func rbDecodePageBody(body []byte) (string, error) {
	r := transform.NewReader(bytes.NewReader(body), charmap.Windows1252.NewDecoder())
	out, err := io.ReadAll(r)
	if err != nil {
		return string(body), nil
	}
	return string(out), nil
}

func rbReadHTMLContent(data []byte, it rbTOCItem) (string, error) {
	if it.Flags == 1 || it.Flags == 2 {
		return "", nil
	}
	off := int(it.Offset)
	if off < 0 || off > len(data) {
		return "", errors.New("rb: смещение страницы вне файла")
	}
	switch it.Flags {
	case 8:
		if off+8 > len(data) {
			return "", errors.New("rb: обрезан заголовок сжатой страницы")
		}
		nChunks := int(binary.LittleEndian.Uint32(data[off:]))
		off += 8 // count + uncompressed size (unused)
		var sb strings.Builder
		for i := 0; i < nChunks; i++ {
			if off+4 > len(data) {
				return "", errors.New("rb: обрезан размер чанка")
			}
			csz := int(binary.LittleEndian.Uint32(data[off:]))
			off += 4
			if csz < 0 || off+csz > len(data) {
				return "", errors.New("rb: обрезаны данные zlib")
			}
			chunk := data[off : off+csz]
			off += csz
			zr, err := zlib.NewReader(bytes.NewReader(chunk))
			if err != nil {
				return "", fmt.Errorf("rb: zlib: %w", err)
			}
			un, err := io.ReadAll(zr)
			_ = zr.Close()
			if err != nil {
				return "", err
			}
			part, err := rbDecodePageBody(un)
			if err != nil {
				return "", err
			}
			sb.WriteString(part)
		}
		return sb.String(), nil
	default:
		sz := int(it.Size)
		if sz < 0 || off+sz > len(data) {
			return "", errors.New("rb: обрезан текст страницы")
		}
		return rbDecodePageBody(data[off : off+sz])
	}
}

// rbToParagraphs извлекает HTML-страницы из Rocket eBook и гонит их через htmlToParagraphs.
func rbToParagraphs(raw []byte) ([]string, error) {
	toc, err := rbReadTOC(raw)
	if err != nil {
		return nil, err
	}
	var parts []string
	for _, it := range toc {
		nl := strings.ToLower(it.Name)
		if !strings.HasSuffix(nl, ".html") && !strings.HasSuffix(nl, ".htm") {
			continue
		}
		html, err := rbReadHTMLContent(raw, it)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(html) == "" {
			continue
		}
		html = strings.ReplaceAll(html, "<TITLE>", "<TITLE> ")
		parts = append(parts, html)
	}
	if len(parts) == 0 {
		return nil, errors.New("rb: в TOC нет HTML-страниц")
	}
	combined := strings.Join(parts, "\n")
	return htmlToParagraphs(combined)
}

// --- PML: strip_pml (упрощённое чтение без полного PML_HTMLizer) ---

var (
	rePMLStripC      = regexp.MustCompile(`\\C\d=".*"`)
	rePMLStripFnEq   = regexp.MustCompile(`\\Fn=".*"`)
	rePMLStripSdEq   = regexp.MustCompile(`\\Sd=".*"`)
	rePMLStripDotEq  = regexp.MustCompile(`\\\.=".*"`)
	rePMLStripX      = regexp.MustCompile(`\\X\d`)
	rePMLStripSpbd   = regexp.MustCompile(`\\S[pbd]`)
	rePMLStripFnBare = regexp.MustCompile(`\\Fn`)
	rePMLStripA3     = regexp.MustCompile(`\\a\d{3}`)
	rePMLStripU4     = regexp.MustCompile(`\\U[0-9a-fA-F]{4}`)
	rePMLStripAnyEsc = regexp.MustCompile(`\\.`)
	rePMLCommentV    = regexp.MustCompile(`(?s)\\v.*?\\v`)
)

// pmlStripToPlainText повторяет strip_pml из pmlconverter.py (порядок важен: «\\.» в конце).
func pmlStripToPlainText(pml string) string {
	s := rePMLCommentV.ReplaceAllString(pml, "")
	s = rePMLStripC.ReplaceAllString(s, "")
	s = rePMLStripFnEq.ReplaceAllString(s, "")
	s = rePMLStripSdEq.ReplaceAllString(s, "")
	s = rePMLStripDotEq.ReplaceAllString(s, "")
	s = rePMLStripX.ReplaceAllString(s, "")
	s = rePMLStripSpbd.ReplaceAllString(s, "")
	s = rePMLStripFnBare.ReplaceAllString(s, "")
	s = rePMLStripA3.ReplaceAllString(s, "")
	s = rePMLStripU4.ReplaceAllString(s, "")
	s = rePMLStripAnyEsc.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func pmlDecodeString(raw []byte) string {
	r := transform.NewReader(bytes.NewReader(raw), charmap.Windows1252.NewDecoder())
	b, err := io.ReadAll(r)
	if err != nil {
		u, _ := decodeBytesToUTF8(raw, "txt")
		return string(u)
	}
	return string(b)
}

func pmlToParagraphs(raw []byte) ([]string, error) {
	plain := pmlStripToPlainText(pmlDecodeString(raw))
	if plain == "" {
		return nil, errors.New("pml: после вычистки разметки текст пуст")
	}
	return paragraphsFromPlainText(plain), nil
}

// pmlzToParagraphs — ZIP с одним или несколькими .pml (как pml_input pmlz).
func pmlzToParagraphs(raw []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("pmlz: не zip: %w", err)
	}
	var names []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(path.Ext(f.Name), ".pml") {
			names = append(names, f.Name)
		}
	}
	if len(names) == 0 {
		return nil, errors.New("pmlz: нет .pml в архиве")
	}
	sort.Strings(names)
	var chunks []string
	for _, n := range names {
		var f *zip.File
		for _, zf := range zr.File {
			if zf.Name == n {
				f = zf
				break
			}
		}
		if f == nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		plain := pmlStripToPlainText(pmlDecodeString(b))
		if plain != "" {
			chunks = append(chunks, plain)
		}
	}
	if len(chunks) == 0 {
		return nil, errors.New("pmlz: не удалось извлечь текст из .pml")
	}
	return paragraphsFromPlainText(strings.Join(chunks, "\n\n")), nil
}
