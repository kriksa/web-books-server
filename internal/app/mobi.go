package app
import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const (
	mobiHuffCDIC = uint16(17480) // 0x4448, KF8 — отдельная распаковка
)

func pdbSplitRecords(data []byte) ([][]byte, error) {
	if len(data) < 78 {
		return nil, errors.New("pdb")
	}
	n := int(binary.BigEndian.Uint16(data[76:78]))
	if n <= 0 || n > 8192 {
		return nil, errors.New("pdb: некорректное число записей")
	}
	if 78+4*n > len(data) {
		return nil, errors.New("pdb: укороченный индекс")
	}
	offs := make([]int, n)
	for i := 0; i < n; i++ {
		offs[i] = int(binary.BigEndian.Uint32(data[78+4*i:]))
	}
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		start := offs[i]
		end := len(data)
		if i+1 < n {
			end = offs[i+1]
		}
		if start < 0 || start > end || end > len(data) {
			return nil, fmt.Errorf("pdb: смещение записи %d", i)
		}
		out[i] = data[start:end]
	}
	return out, nil
}

// PalmDOC (Mobipocket) — см. https://wiki.mobileread.com/wiki/PalmDOC
func decompressPalmDOC(comp []byte) []byte {
	out := make([]byte, 0, len(comp)*2)
	i := 0
	for i < len(comp) {
		c := comp[i]
		i++
		switch {
		case c == 0:
			out = append(out, 0)
		case c >= 1 && c <= 8:
			if i+int(c) > len(comp) {
				return out
			}
			out = append(out, comp[i:i+int(c)]...)
			i += int(c)
		case c >= 9 && c <= 0x7f:
			out = append(out, c)
		case c >= 0xc0 && c <= 0xff:
			out = append(out, ' ')
			out = append(out, c^0x80)
		case c >= 0x80 && c <= 0xbf:
			if i >= len(comp) {
				return out
			}
			c2 := comp[i]
			i++
			bits := (uint16(c&0x3f) << 8) | uint16(c2)
			lzlen := int(bits&7) + 3
			dist := int(bits>>3) & 0x7ff
			if dist == 0 || dist > len(out) {
				continue
			}
			start := len(out) - dist
			for j := 0; j < lzlen; j++ {
				out = append(out, out[start+j])
			}
		default:
			// не по спецификации — игнорируем
		}
	}
	return out
}

// mobiRawHTMLBytes извлекает сжатый HTML/текст (PalmDOC, без сжатия, Huff/CDIC / KF8).
func mobiRawHTMLBytes(raw []byte) ([]byte, []byte, error) {
	recs, err := pdbSplitRecords(raw)
	if err != nil {
		return nil, nil, err
	}
	ah, err := resolveMobiActiveHeader(recs)
	if err != nil {
		return nil, nil, err
	}
	switch ah.Compression {
	case mobiHuffCDIC:
		blob, err := mobiUnpackHuffCDIC(recs, ah)
		if err != nil {
			return nil, nil, err
		}
		return blob, ah.Raw, nil
	case 1, 2:
		textEnd := ah.TextStart + int(ah.Records)
		if textEnd > len(recs) {
			textEnd = len(recs)
		}
		var blob []byte
		for i := ah.TextStart; i < textEnd; i++ {
			sec := recs[i]
			tail := mobiTrailingSize(sec, ah.ExtraFlags)
			if tail > len(sec) {
				tail = 0
			}
			sec = sec[:len(sec)-tail]
			if ah.Compression == 1 {
				blob = append(blob, sec...)
			} else {
				blob = append(blob, decompressPalmDOC(sec)...)
			}
		}
		if len(blob) == 0 {
			return nil, nil, errors.New("mobi: пустой текст")
		}
		return blob, ah.Raw, nil
	default:
		return nil, nil, fmt.Errorf("mobi: неизвестное сжатие %d", ah.Compression)
	}
}

func mobiEncodingPage(hdr []byte) uint32 {
	if len(hdr) >= 28 {
		return binary.BigEndian.Uint32(hdr[24:28])
	}
	return 1252
}

func mobiBytesToUTF8(blob []byte, hdr []byte) string {
	if utf8.Valid(blob) {
		return string(blob)
	}
	switch mobiEncodingPage(hdr) {
	case 65001:
		return string(blob)
	case 1251:
		u, e := io.ReadAll(transform.NewReader(bytes.NewReader(blob), charmap.Windows1251.NewDecoder()))
		if e == nil && len(u) > 0 {
			return string(u)
		}
	}
	u, err := io.ReadAll(transform.NewReader(bytes.NewReader(blob), charmap.Windows1252.NewDecoder()))
	if err == nil && len(u) > 0 {
		return string(u)
	}
	return string(blob)
}

func mobiToParagraphs(raw []byte) ([]string, error) {
	blob, hdr, err := mobiRawHTMLBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("mobi: %w", err)
	}
	s := mobiBytesToUTF8(blob, hdr)
	prefix := blob
	if len(prefix) > 4096 {
		prefix = prefix[:4096]
	}
	low := bytes.ToLower(prefix)
	if bytes.Contains(low, []byte("<html")) || bytes.Contains(low, []byte("<xml")) ||
		bytes.Contains(low, []byte("<body")) || bytes.Contains(low, []byte("<p>")) {
		utf8HTML, _ := decodeBytesToUTF8([]byte(s), "html")
		p, err := htmlToParagraphs(string(utf8HTML))
		if err == nil && len(p) > 0 {
			return p, nil
		}
	}
	p := paragraphsFromPlainText(s)
	if len(p) == 0 {
		return nil, errors.New("mobi: не удалось получить абзацы")
	}
	return p, nil
}
