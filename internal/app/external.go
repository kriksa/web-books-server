package app
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const externalConvertTimeout = 90 * time.Second
const djvuConvertTimeout = 4 * time.Minute

func runTextExtractor(name string, args []string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), externalConvertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.Output()
}

func runTextExtractorTimeout(name string, timeout time.Duration, args []string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.Output()
}

func djvuTryExtract(path string) ([]byte, error) {
	// 1) djvutxt — скрытый текстовый слой (весь документ).
	if out, err := runTextExtractorTimeout("djvutxt", djvuConvertTimeout, []string{path}); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return out, nil
		}
	}
	// 2) djvutxt -detail=lines — явная разбивка по строкам (некоторые издания).
	if out, err := runTextExtractorTimeout("djvutxt", djvuConvertTimeout, []string{"-detail=lines", path}); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return out, nil
		}
	}
	// 3) djvutxt -e — непрерывный «плоский» текст.
	if out, err := runTextExtractorTimeout("djvutxt", djvuConvertTimeout, []string{"-e", path}); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return out, nil
		}
	}
	return nil, errors.New("djvu: установите djvulibre (djvutxt) и добавьте в PATH")
}

func djvuToParagraphs(raw []byte) ([]string, error) {
	f, err := os.CreateTemp("", "wb-conv-*.djvu")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	_ = f.Close()
	defer os.Remove(path)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	out, err := djvuTryExtract(path)
	if err != nil {
		return nil, err
	}
	// ddjvu может слать UTF-16 BOM или смесь — нормализуем в UTF-8 строку.
	s := strings.TrimSpace(string(bytes.TrimPrefix(out, []byte{0xef, 0xbb, 0xbf})))
	if s == "" {
		return nil, errors.New("djvu: пустой текст после извлечения")
	}
	return paragraphsFromPlainText(s), nil
}

func legacyDocToParagraphs(raw []byte) ([]string, error) {
	f, err := os.CreateTemp("", "wb-conv-*.doc")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	_ = f.Close()
	defer os.Remove(path)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, err
	}
	var out []byte
	var tried error
	for _, exe := range []string{"antiword", "catdoc"} {
		b, e := runTextExtractor(exe, []string{path})
		if e != nil {
			tried = e
			continue
		}
		if strings.TrimSpace(string(b)) != "" {
			out = b
			tried = nil
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("doc: установите antiword или catdoc в PATH (%v)", tried)
	}
	s := strings.TrimSpace(string(out))
	return paragraphsFromPlainText(s), nil
}
