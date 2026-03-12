package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/timsims/pamphlet"
	"golang.org/x/text/encoding/charmap"
)

// utils.go — вспомогательные функции (формат файла, MIME, санитайз имени, парсинг метаданных FB2/EPUB).
//
// Здесь собрано то, что используется в разных слоях (HTTP-обработчики, OPDS, извлечение обложек, детали книг).
// Важно: некоторые функции работают с входными данными “как есть” (архивы/FB2/EPUB),
// поэтому они должны быть устойчивыми к неожиданным форматам и кодировкам.

func ensureFormat(format, fileName string) string {
	if format == "" || format == "unknown" {
		return determineFormatFromFileName(fileName)
	}
	return format
}

func determineFormatFromFileName(fileName string) string {
	fileName = strings.ToLower(fileName)

	formatMap := map[string]string{
		".fb2":  "fb2",
		".epub": "epub",
		".mobi": "mobi",
		".pdf":  "pdf",
		".djvu": "djvu",
		".txt":  "txt",
		".doc":  "doc",
		".docx": "docx",
		".rtf":  "rtf",
		".html": "html",
		".htm":  "html",
	}

	for ext, format := range formatMap {
		if strings.HasSuffix(fileName, ext) {
			return format
		}
	}

	if strings.Contains(fileName, "fb2") {
		return "fb2"
	}
	if strings.Contains(fileName, "epub") {
		return "epub"
	}
	if strings.Contains(fileName, "mobi") {
		return "mobi"
	}
	if strings.Contains(fileName, "pdf") {
		return "pdf"
	}
	if strings.Contains(fileName, "djvu") {
		return "djvu"
	}

	return "unknown"
}

func sanitizeFilename(name string) string {
	// sanitizeFilename удаляет символы, запрещённые в файловых именах (особенно важно для Windows),
	// и убирает управляющие символы.
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func mimeForFormat(format string) string {
	format = strings.ToLower(format)
	if mime, ok := supportedFormats[format]; ok {
		return mime
	}
	return "application/octet-stream"
}

func ParseFB2Metadata(content []byte) (*FB2, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = makeCharsetReader

	var fb2 FB2
	err := decoder.Decode(&fb2)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга XML: %v", err)
	}

	return &fb2, nil
}

func makeCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	charset = strings.ToLower(charset)
	switch charset {
	case "windows-1251", "cp1251", "win-1251":
		return charmap.Windows1251.NewDecoder().Reader(input), nil
	case "koi8-r":
		return charmap.KOI8R.NewDecoder().Reader(input), nil
	case "iso-8859-5":
		return charmap.ISO8859_5.NewDecoder().Reader(input), nil
	case "utf-8", "utf8":
		return input, nil
	default:
		return input, nil
	}
}

func CleanAnnotation(xmlContent string) string {
	if xmlContent == "" {
		return ""
	}

	replacements := map[string]string{
		`<subtitle>`:   `<p><b>`,
		`</subtitle>`:  `</b></p>`,
		`<empty-line/>`: `<br>`,
		`<empty-line />`: `<br>`,
		`<strong>`:     `<b>`,
		`</strong>`:    `</b>`,
		`<emphasis>`:   `<i>`,
		`</emphasis>`:  `</i>`,
		`<stanza>`:     `<br>`,
		`</stanza>`:    ``,
		`<poem>`:       `<br>`,
		`</poem>`:      ``,
		`<cite>`:       `<i>`,
		`</cite>`:      `</i>`,
		`<table>`:      `<br>`,
		`</table>`:     ``,
		`<p>`:          `<p>`,
		`</p>`:         `</p>`,
		`&lt;`:         `<`,
		`&gt;`:         `>`,
		`&amp;`:        `&`,
	}

	result := xmlContent
	for oldTag, newTag := range replacements {
		result = strings.ReplaceAll(result, oldTag, newTag)
	}

	reNS := regexp.MustCompile(`\sxmlns="[^"]+"`)
	result = reNS.ReplaceAllString(result, "")

	return strings.TrimSpace(result)
}

func ExtractDetailedInfo(fb2 *FB2) DetailedBookInfo {
	info := DetailedBookInfo{
		TitleInfo:    make(map[string]interface{}),
		SrcTitleInfo: make(map[string]interface{}),
		PublishInfo:  make(map[string]interface{}),
		DocumentInfo: make(map[string]interface{}),
	}

	formatAuthors := func(authors []FB2Author) []string {
		res := []string{}
		for _, a := range authors {
			res = append(res, a.String())
		}
		return res
	}

	formatSequence := func(seqs []FB2Sequence) []string {
		res := []string{}
		for _, s := range seqs {
			val := s.Name
			if s.Number > 0 {
				val = fmt.Sprintf("%s #%d", s.Name, s.Number)
			}
			res = append(res, val)
		}
		return res
	}

	ti := fb2.Description.TitleInfo
	info.TitleInfo["bookTitle"] = ti.BookTitle
	info.TitleInfo["genre"] = ti.Genre
	info.TitleInfo["author"] = formatAuthors(ti.Author)
	info.TitleInfo["annotationHtml"] = CleanAnnotation(ti.Annotation.Content)
	info.TitleInfo["keywords"] = ti.Keywords
	info.TitleInfo["date"] = ti.Date
	info.TitleInfo["lang"] = ti.Lang
	info.TitleInfo["srcLang"] = ti.SrcLang
	info.TitleInfo["translator"] = formatAuthors(ti.Translator)
	info.TitleInfo["sequence"] = formatSequence(ti.Sequence)

	sti := fb2.Description.SrcTitleInfo
	if sti.BookTitle != "" {
		info.SrcTitleInfo["bookTitle"] = sti.BookTitle
		info.SrcTitleInfo["author"] = formatAuthors(sti.Author)
		info.SrcTitleInfo["date"] = sti.Date
		info.SrcTitleInfo["lang"] = sti.Lang
		info.SrcTitleInfo["sequence"] = formatSequence(sti.Sequence)
	}

	pi := fb2.Description.PublishInfo
	if pi.BookName != "" || pi.Publisher != "" {
		info.PublishInfo["bookName"] = pi.BookName
		info.PublishInfo["publisher"] = pi.Publisher
		info.PublishInfo["city"] = pi.City
		info.PublishInfo["year"] = pi.Year
		info.PublishInfo["isbn"] = pi.ISBN
		info.PublishInfo["sequence"] = formatSequence(pi.Sequence)
	}

	di := fb2.Description.DocumentInfo
	info.DocumentInfo["author"] = formatAuthors(di.Author)
	info.DocumentInfo["programUsed"] = di.ProgramUsed
	info.DocumentInfo["date"] = di.Date
	info.DocumentInfo["id"] = di.ID
	info.DocumentInfo["version"] = di.Version
	info.DocumentInfo["srcOcr"] = di.SrcOcr
	info.DocumentInfo["historyHtml"] = CleanAnnotation(di.History.Content)

	return info
}

func ConvertPamphletToDetails(book *pamphlet.Book) DetailedBookInfo {
	info := DetailedBookInfo{
		TitleInfo:    make(map[string]interface{}),
		SrcTitleInfo: make(map[string]interface{}),
		PublishInfo:  make(map[string]interface{}),
		DocumentInfo: make(map[string]interface{}),
	}

	if book.Title != "" {
		info.TitleInfo["bookTitle"] = book.Title
	}

	if book.Author != "" {
		info.TitleInfo["author"] = []string{book.Author}
	}

	if book.Description != "" {
		info.TitleInfo["annotationHtml"] = CleanAnnotation(book.Description)
	}

	if book.Language != "" {
		info.TitleInfo["lang"] = book.Language
	}

	if book.Publisher != "" {
		info.PublishInfo["publisher"] = book.Publisher
	}

	if book.Date != "" {
		info.PublishInfo["year"] = book.Date
	}

	if book.Identifier != "" {
		info.PublishInfo["isbn"] = book.Identifier
	}

	if book.Subject != "" {
		info.TitleInfo["keywords"] = book.Subject
	}

	return info
}