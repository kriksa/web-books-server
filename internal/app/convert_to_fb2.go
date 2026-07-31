package app

import (
	"bytes"
	"fmt"
	"html"
	"strings"
	"time"
)

const maxConvertInputBytes = 64 << 20 // 64 MiB

func fb2LooksLikeFictionBook(raw []byte) bool {
	sample := raw
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	lower := strings.ToLower(string(sample))
	return strings.Contains(lower, "<fictionbook") ||
		strings.Contains(lower, "fictionbook/2.0") ||
		strings.Contains(lower, "gribuser.ru/xml/fictionbook")
}

func nativeConvertCacheKey(raw []byte, format string) string {
	return sha256Hex(append([]byte(strings.ToLower(strings.TrimSpace(format))+"|"), raw...))
}

func convertRawToFB2UTF8(raw []byte, format, title, author string) ([]byte, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "fb2" && fb2LooksLikeFictionBook(raw) {
		out, _ := decodeBytesToUTF8(raw, "fb2")
		return replaceFB2XmlEncodingToUTF8(out), nil
	}
	paragraphs, err := rawFormatToParagraphs(raw, format)
	if err != nil {
		return nil, err
	}
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("не удалось извлечь текст из %s", format)
	}
	return buildFB2XML(title, author, paragraphs), nil
}

func rawFormatToParagraphs(raw []byte, format string) ([]string, error) {
	switch format {
	case "txt":
		s, _ := decodeBytesToUTF8(raw, format)
		return paragraphsFromPlainText(string(s)), nil
	case "html", "htm":
		s, _ := decodeBytesToUTF8(raw, format)
		_, pars, _, _ := extractXhtmlText(s, "html-1")
		return pars, nil
	case "docx":
		return docxToParagraphsFlat(raw)
	case "epub":
		return epubToParagraphsFlat(raw)
	case "rtf":
		return rtfToParagraphs(raw)
	case "doc":
		return legacyDocToParagraphs(raw)
	case "djvu":
		return djvuToParagraphs(raw)
	case "mobi":
		return mobiToParagraphs(raw)
	case "tcr":
		return tcrToParagraphs(raw)
	case "odt":
		return odtToParagraphs(raw)
	case "htmlz":
		return htmlzToParagraphs(raw)
	case "rb":
		return rbToParagraphs(raw)
	case "pml":
		return pmlToParagraphs(raw)
	case "pmlz":
		return pmlzToParagraphs(raw)
	default:
		return nil, fmt.Errorf("конвертация в FB2 для формата %q не поддерживается", format)
	}
}

func docxToParagraphsFlat(raw []byte) ([]string, error) {
	model := &ReaderRenderModel{}
	if err := fillDocxModel(raw, model); err != nil {
		return nil, err
	}
	return flattenRenderParagraphs(model.Sections), nil
}

func epubToParagraphsFlat(raw []byte) ([]string, error) {
	model := &ReaderRenderModel{}
	if err := fillEpubModel(raw, model); err != nil {
		return nil, err
	}
	return flattenRenderParagraphs(model.Sections), nil
}

func flattenRenderParagraphs(sections []ReaderRenderSection) []string {
	var out []string
	for _, sec := range sections {
		for _, p := range sec.Paragraphs {
			if t := cleanText(p); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func buildFB2XML(title, author string, paragraphs []string) []byte {
	title = firstNonEmptyClean(title, "Без названия")
	author = firstNonEmptyClean(author, "Неизвестный автор")
	docID := fmt.Sprintf("web-books-%d", time.Now().UnixNano())
	date := time.Now().Format("2006-01-02")

	var body strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		body.WriteString("<p>")
		body.WriteString(xmlEscapeText(p))
		body.WriteString("</p>\n")
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	buf.WriteString(`<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0" xmlns:l="http://www.w3.org/1999/xlink">` + "\n")
	buf.WriteString("<description>\n<title-info>\n")
	buf.WriteString("<author><nickname>")
	buf.WriteString(xmlEscapeText(author))
	buf.WriteString("</nickname></author>\n")
	buf.WriteString("<book-title>")
	buf.WriteString(xmlEscapeText(title))
	buf.WriteString("</book-title>\n</title-info>\n")
	buf.WriteString("<document-info>\n<author><nickname>web-books</nickname></author>\n")
	buf.WriteString(`<date value="`)
	buf.WriteString(date)
	buf.WriteString(`">`)
	buf.WriteString(date)
	buf.WriteString("</date>\n<id>")
	buf.WriteString(xmlEscapeText(docID))
	buf.WriteString("</id>\n<version>1.0</version>\n</document-info>\n</description>\n")
	buf.WriteString("<body>\n<title><p>")
	buf.WriteString(xmlEscapeText(title))
	buf.WriteString("</p></title>\n<section>\n")
	buf.WriteString(body.String())
	buf.WriteString("</section>\n</body>\n</FictionBook>\n")
	return buf.Bytes()
}

func xmlEscapeText(s string) string {
	return html.EscapeString(s)
}
