package bookutil

import (
	"strings"
	"unicode"
)

var supportedFormats = map[string]string{
	"fb2":   "application/fb2",
	"epub":  "application/epub+zip",
	"mobi":  "application/x-mobipocket-ebook",
	"pdf":   "application/pdf",
	"djvu":  "image/vnd.djvu",
	"txt":   "text/plain",
	"doc":   "application/msword",
	"docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"rtf":   "application/rtf",
	"html":  "text/html",
	"htm":   "text/html",
	"odt":   "application/vnd.oasis.opendocument.text",
	"htmlz": "application/zip",
	"tcr":   "application/octet-stream",
	"rb":    "application/octet-stream",
	"pml":   "application/octet-stream",
	"pmlz":  "application/zip",
}

func EnsureFormat(format, fileName string) string {
	if format == "" || format == "unknown" {
		return DetermineFormatFromFileName(fileName)
	}
	return format
}

func DetermineFormatFromFileName(fileName string) string {
	fileName = strings.ToLower(fileName)
	formatMap := map[string]string{
		".fb2": "fb2", ".epub": "epub", ".mobi": "mobi", ".azw": "azw", ".azw3": "azw3",
		".prc": "prc", ".pdf": "pdf", ".djvu": "djvu", ".txt": "txt", ".doc": "doc",
		".docx": "docx", ".rtf": "rtf", ".html": "html", ".htm": "html", ".odt": "odt",
		".htmlz": "htmlz", ".tcr": "tcr", ".rb": "rb", ".pml": "pml", ".pmlz": "pmlz",
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

func SanitizeFilename(name string) string {
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

func MimeForFormat(format string) string {
	format = strings.ToLower(format)
	if mime, ok := supportedFormats[format]; ok {
		return mime
	}
	return "application/octet-stream"
}
