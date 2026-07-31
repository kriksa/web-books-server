package app
import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"sync"

	"web_books/internal/formats/docx"
)

type ReaderTOCItem struct {
	Label    string          `json:"label"`
	Href     string          `json:"href,omitempty"`
	Page     int             `json:"page,omitempty"`
	Fraction float64         `json:"fraction,omitempty"`
	Subitems []ReaderTOCItem `json:"subitems,omitempty"`
}

var tocCache sync.Map

func cacheKey(format string, raw []byte) string {
	head := raw
	if len(head) > 1024 {
		head = head[:1024]
	}
	return strings.ToLower(format) + ":" + string(head) + ":" + string(rune(len(raw)%65535))
}

func extractServerTOC(format string, raw []byte) []ReaderTOCItem {
	key := cacheKey(format, raw)
	if v, ok := tocCache.Load(key); ok {
		if cached, ok2 := v.([]ReaderTOCItem); ok2 {
			return cached
		}
	}
	var out []ReaderTOCItem
	switch strings.ToLower(format) {
	case "txt":
		out = buildTxtTOC(raw)
	case "docx":
		out = buildDocxTOC(raw)
	case "fb2":
		out = buildFB2TOC(raw)
	case "epub":
		out = buildEpubTOC(raw)
	default:
		out = nil
	}
	tocCache.Store(key, out)
	return out
}

func buildTxtTOC(raw []byte) []ReaderTOCItem {
	text := string(raw)
	parts := strings.Split(text, "\n")
	total := len(parts)
	if total < 40 {
		return nil
	}
	step := 200
	if total > 4000 {
		step = 350
	}
	out := make([]ReaderTOCItem, 0, total/step+1)
	for i := 0; i < total; i += step {
		f := float64(i) / float64(total)
		label := strings.TrimSpace(parts[i])
		if label == "" {
			label = "Раздел"
		}
		if len(label) > 64 {
			label = label[:64] + "..."
		}
		out = append(out, ReaderTOCItem{
			Label:    "Часть: " + label,
			Fraction: f,
		})
	}
	return out
}

func buildDocxTOC(raw []byte) []ReaderTOCItem {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil
	}
	docXML, err := docx.OpenDocumentXML(zr)
	if err != nil || len(docXML) == 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(docXML))
	var inP, heading bool
	var headingCount int
	var textBuf strings.Builder
	type entry struct {
		label string
		idx   int
	}
	entries := make([]entry, 0, 64)
	pIndex := 0
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
				pIndex++
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
			name := strings.ToLower(t.Name.Local)
			if name == "p" {
				if heading {
					txt := strings.TrimSpace(textBuf.String())
					if txt != "" {
						if len(txt) > 80 {
							txt = txt[:80] + "..."
						}
						entries = append(entries, entry{label: txt, idx: pIndex})
						headingCount++
					}
				}
				inP = false
			}
		case xml.CharData:
			if inP {
				textBuf.WriteString(string(t))
			}
		}
	}
	if headingCount == 0 {
		return nil
	}
	total := pIndex
	out := make([]ReaderTOCItem, 0, len(entries))
	for _, e := range entries {
		f := 0.0
		if total > 0 {
			f = float64(e.idx) / float64(total)
		}
		out = append(out, ReaderTOCItem{Label: e.label, Fraction: f})
	}
	return out
}

func buildFB2TOC(raw []byte) []ReaderTOCItem {
	s := string(raw)
	re := regexp.MustCompile(`(?is)<section\b[^>]*>.*?<title\b[^>]*>\s*(?:<p\b[^>]*>)?\s*([^<]+?)\s*(?:</p>)?\s*</title>`)
	matches := re.FindAllStringSubmatch(s, 80)
	if len(matches) == 0 {
		return nil
	}
	total := len(matches)
	out := make([]ReaderTOCItem, 0, total)
	for i, m := range matches {
		label := strings.TrimSpace(m[1])
		if label == "" {
			label = "Глава"
		}
		if len(label) > 90 {
			label = label[:90] + "..."
		}
		f := float64(i) / float64(total)
		out = append(out, ReaderTOCItem{Label: label, Fraction: f})
	}
	return out
}

func buildEpubTOC(raw []byte) []ReaderTOCItem {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil
	}
	var navData []byte
	var ncxData []byte
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "nav") && (strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".html")) {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(strings.ToLower(string(b)), "toc") {
				navData = b
				break
			}
		}
		if strings.HasSuffix(name, ".ncx") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			ncxData, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if len(navData) == 0 && len(ncxData) == 0 {
		return buildEpubSpineFallback(zr)
	}
	if len(navData) > 0 {
		re := regexp.MustCompile(`(?is)<a[^>]*href="([^"]+)"[^>]*>\s*([^<][^<]*?)\s*</a>`)
		matches := re.FindAllStringSubmatch(string(navData), 200)
		if len(matches) > 0 {
			out := make([]ReaderTOCItem, 0, len(matches))
			for _, m := range matches {
				href := strings.TrimSpace(m[1])
				label := strings.TrimSpace(m[2])
				if href == "" || label == "" {
					continue
				}
				if len(label) > 90 {
					label = label[:90] + "..."
				}
				out = append(out, ReaderTOCItem{Label: label, Href: href})
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	if len(ncxData) > 0 {
		re := regexp.MustCompile(`(?is)<navPoint[^>]*>.*?<text>\s*([^<]+)\s*</text>.*?<content[^>]*src="([^"]+)"`)
		matches := re.FindAllStringSubmatch(string(ncxData), 200)
		out := make([]ReaderTOCItem, 0, len(matches))
		for _, m := range matches {
			label := strings.TrimSpace(m[1])
			href := strings.TrimSpace(m[2])
			if label == "" || href == "" {
				continue
			}
			out = append(out, ReaderTOCItem{Label: label, Href: href})
		}
		if len(out) > 0 {
			return out
		}
	}
	return buildEpubSpineFallback(zr)
}

func buildEpubSpineFallback(zr *zip.Reader) []ReaderTOCItem {
	items := make([]ReaderTOCItem, 0, 160)
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".html") {
			base := f.Name
			if idx := strings.LastIndexAny(base, `/\`); idx >= 0 && idx < len(base)-1 {
				base = base[idx+1:]
			}
			items = append(items, ReaderTOCItem{Label: base, Href: f.Name})
			if len(items) >= 160 {
				break
			}
		}
	}
	return items
}
