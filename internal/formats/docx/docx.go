package docx

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"
)

// OpenDocumentXML возвращает содержимое word/document.xml из DOCX.
func OpenDocumentXML(zr *zip.Reader) ([]byte, error) {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, "word/document.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("docx: word/document.xml not found")
}
