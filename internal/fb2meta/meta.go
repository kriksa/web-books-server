package fb2meta

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/timsims/pamphlet"
	"golang.org/x/text/encoding/charmap"

	"web_books/internal/domain"
)

func Parse(content []byte) (*domain.FB2, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = charsetReader
	var fb2 domain.FB2
	if err := decoder.Decode(&fb2); err != nil {
		return nil, fmt.Errorf("ошибка парсинга XML: %v", err)
	}
	return &fb2, nil
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "windows-1251", "cp1251", "win-1251":
		return charmap.Windows1251.NewDecoder().Reader(input), nil
	case "koi8-r":
		return charmap.KOI8R.NewDecoder().Reader(input), nil
	case "iso-8859-5":
		return charmap.ISO8859_5.NewDecoder().Reader(input), nil
	default:
		return input, nil
	}
}

func CleanAnnotation(xmlContent string) string {
	if xmlContent == "" {
		return ""
	}
	replacements := map[string]string{
		`<subtitle>`: `<p><b>`, `</subtitle>`: `</b></p>`,
		`<empty-line/>`: `<br>`, `<empty-line />`: `<br>`,
		`<strong>`: `<b>`, `</strong>`: `</b>`,
		`<emphasis>`: `<i>`, `</emphasis>`: `</i>`,
		`&lt;`: `<`, `&gt;`: `>`, `&amp;`: `&`,
	}
	result := xmlContent
	for oldTag, newTag := range replacements {
		result = strings.ReplaceAll(result, oldTag, newTag)
	}
	reNS := regexp.MustCompile(`\sxmlns="[^"]+"`)
	return strings.TrimSpace(reNS.ReplaceAllString(result, ""))
}

func Detailed(fb2 *domain.FB2) domain.DetailedBookInfo {
	info := domain.DetailedBookInfo{
		TitleInfo:    make(map[string]interface{}),
		SrcTitleInfo: make(map[string]interface{}),
		PublishInfo:  make(map[string]interface{}),
		DocumentInfo: make(map[string]interface{}),
	}
	formatAuthors := func(authors []domain.FB2Author) []string {
		res := []string{}
		for _, a := range authors {
			res = append(res, a.String())
		}
		return res
	}
	formatSequence := func(seqs []domain.FB2Sequence) []string {
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

func FromPamphlet(book *pamphlet.Book) domain.DetailedBookInfo {
	info := domain.DetailedBookInfo{
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

func CoverBytes(fb2 *domain.FB2) ([]byte, string, error) {
	if fb2 == nil {
		return nil, "", domain.ErrCoverNotFound
	}
	href := strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.XLinkHref)
	if href == "" {
		href = strings.TrimSpace(fb2.Description.TitleInfo.Coverpage.Image.Href)
	}
	href = strings.TrimPrefix(href, "#")
	if href == "" {
		return nil, "", domain.ErrCoverNotFound
	}
	for _, b := range fb2.Binary {
		if strings.TrimSpace(b.Id) != href {
			continue
		}
		data := strings.Join(strings.Fields(b.Data), "")
		if data == "" {
			return nil, "", domain.ErrCoverNotFound
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", err
		}
		ct := strings.TrimSpace(b.ContentType)
		if ct == "" {
			ct = "image/jpeg"
		}
		return raw, ct, nil
	}
	return nil, "", domain.ErrCoverNotFound
}
