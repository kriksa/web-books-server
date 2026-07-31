package storage

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"web_books/internal/bookutil"
)

// FindZipEntry locates a file inside an archive by name/format.
func FindZipEntry(z *zip.ReadCloser, fileName, format string) *zip.File {
	targetName := fmt.Sprintf("%s.%s", fileName, format)
	for _, f := range z.File {
		if strings.EqualFold(f.Name, targetName) {
			return f
		}
	}
	for _, f := range z.File {
		if strings.EqualFold(f.Name, fileName) {
			return f
		}
	}
	return nil
}

// ExtractBookBytes reads a book file from its INP zip archive.
func (dm *Manager) ExtractBookBytes(booksDir string, bookID int) (data []byte, format string, err error) {
	fileName, zipName, fmtDB, _, _, _, del, err := dm.GetBookDownloadInfo(bookID)
	if err != nil {
		return nil, "", err
	}
	if del == 1 {
		return nil, "", fmt.Errorf("book deleted")
	}
	format = bookutil.EnsureFormat(fmtDB, fileName)
	zipPath := filepath.Join(booksDir, zipName)
	zf, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer zf.Close()

	targetWithExt := fileName
	if format != "" && !strings.HasSuffix(strings.ToLower(fileName), "."+strings.ToLower(format)) {
		targetWithExt = fmt.Sprintf("%s.%s", fileName, format)
	}

	var target *zip.File
	for _, f := range zf.File {
		if strings.EqualFold(f.Name, targetWithExt) || strings.EqualFold(f.Name, fileName) {
			target = f
			break
		}
	}
	if target == nil {
		base1 := filepath.Base(targetWithExt)
		base2 := filepath.Base(fileName)
		for _, f := range zf.File {
			b := filepath.Base(f.Name)
			if strings.EqualFold(b, base1) || strings.EqualFold(b, base2) {
				target = f
				break
			}
		}
	}
	if target == nil {
		return nil, "", fmt.Errorf("file not found in zip")
	}
	rc, err := target.Open()
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	data, err = io.ReadAll(rc)
	return data, format, err
}
