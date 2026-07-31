package httpapi

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/timsims/pamphlet"
	"golang.org/x/crypto/bcrypt"

	"web_books/internal/bookutil"
	"web_books/internal/config"
	"web_books/internal/domain"
	"web_books/internal/fb2meta"
	"web_books/internal/formats/epub"
	"web_books/internal/storage"
)

const webAuthCookieName = "web_auth_session"

func apiSearchHandler(_ Host, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		filters := domain.SearchFilters{
			Title:    strings.TrimSpace(r.URL.Query().Get("title")),
			Author:   strings.TrimSpace(r.URL.Query().Get("author")),
			Series:   strings.TrimSpace(r.URL.Query().Get("series")),
			Genre:    strings.TrimSpace(r.URL.Query().Get("genre")),
			Language: strings.TrimSpace(r.URL.Query().Get("language")),
		}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 10000)
		if err != nil {
			log.Printf("api/search: %v", err)
			http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"books": books})
	}
}

func apiBookDetailsHandler(dm *storage.Manager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		bookID, err := strconv.Atoi(r.URL.Query().Get("id"))
		if err != nil || bookID <= 0 {
			http.Error(w, "Некорректный id", http.StatusBadRequest)
			return
		}
		raw, format, err := dm.ExtractBookBytes(booksDir, bookID)
		if err != nil {
			if err == sql.ErrNoRows || strings.Contains(err.Error(), "deleted") {
				http.Error(w, "Книга не найдена", http.StatusNotFound)
				return
			}
			log.Printf("api/book/details: %v", err)
			http.Error(w, "Не удалось прочитать книгу", http.StatusInternalServerError)
			return
		}
		format = strings.ToLower(format)
		var info domain.DetailedBookInfo
		switch format {
		case "fb2":
			fb2, err := fb2meta.Parse(raw)
			if err != nil {
				http.Error(w, "Ошибка разбора FB2", http.StatusInternalServerError)
				return
			}
			info = fb2meta.Detailed(fb2)
		case "epub":
			parser, err := pamphlet.OpenBytes(raw)
			if err != nil {
				http.Error(w, "Ошибка разбора EPUB", http.StatusInternalServerError)
				return
			}
			defer parser.Close()
			info = fb2meta.FromPamphlet(parser.GetBook())
		default:
			http.Error(w, "Детали доступны только для FB2 и EPUB", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(info)
	}
}

func apiCoverHandler(dm *storage.Manager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		fileName := r.URL.Query().Get("file")
		zipName := r.URL.Query().Get("zip")
		format := bookutil.EnsureFormat(r.URL.Query().Get("format"), fileName)
		if fileName == "" || zipName == "" {
			http.Error(w, "Параметры file и zip обязательны", http.StatusBadRequest)
			return
		}
		cacheKey := fileName + "|" + zipName + "|" + format
		if cached, ok := imageCache.Get(cacheKey); ok {
			if len(cached) == 0 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", http.DetectContentType(cached))
			w.Header().Set("Cache-Control", "public, max-age=86400")
			_, _ = w.Write(cached)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), domain.CoverExtractTimeout)
		defer cancel()

		type result struct {
			data []byte
			mime string
			err  error
		}
		ch := make(chan result, 1)
		go func() {
			data, mime, err := extractCoverFromArchive(booksDir, zipName, fileName, format)
			ch <- result{data, mime, err}
		}()

		var res result
		select {
		case <-ctx.Done():
			http.Error(w, "Таймаут извлечения обложки", http.StatusGatewayTimeout)
			return
		case res = <-ch:
		}

		if res.err != nil {
			imageCache.Add(cacheKey, missingCoverMarker)
			http.NotFound(w, r)
			return
		}
		imageCache.Add(cacheKey, res.data)
		w.Header().Set("Content-Type", res.mime)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(res.data)
	}
}

func extractCoverFromArchive(booksDir, zipName, fileName, format string) ([]byte, string, error) {
	zipPath := filepath.Join(booksDir, zipName)
	zf, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer zf.Close()

		zfFile := storage.FindZipEntry(zf, fileName, format)
	if zfFile == nil {
		return nil, "", domain.ErrCoverNotFound
	}
	rc, err := zfFile.Open()
	if err != nil {
		return nil, "", err
	}
	raw, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, "", err
	}

	switch strings.ToLower(format) {
	case "fb2":
		fb2, err := fb2meta.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		return fb2meta.CoverBytes(fb2)
	case "epub":
		return epub.ExtractCover(raw)
	default:
		return nil, "", domain.ErrCoverNotFound
	}
}

func downloadHandler(dm *storage.Manager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		pathPart := strings.TrimPrefix(r.URL.Path, "/download/")
		if pathPart == "" {
			http.NotFound(w, r)
			return
		}
		segments := strings.SplitN(pathPart, "/", 2)
		bookID, err := strconv.Atoi(segments[0])
		if err != nil || bookID <= 0 {
			http.NotFound(w, r)
			return
		}

		fileName, zipName, format, title, _, author, del, err := dm.GetBookDownloadInfo(bookID)
		if err == sql.ErrNoRows || del == 1 {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Ошибка базы данных", http.StatusInternalServerError)
			return
		}
		format = bookutil.EnsureFormat(format, fileName)

		zipPath := filepath.Join(booksDir, zipName)
		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			http.Error(w, "Архив недоступен", http.StatusInternalServerError)
			return
		}
		defer zf.Close()

		zfFile := storage.FindZipEntry(zf, fileName, format)
		if zfFile == nil {
			http.NotFound(w, r)
			return
		}
		rc, err := zfFile.Open()
		if err != nil {
			http.Error(w, "Не удалось открыть файл", http.StatusInternalServerError)
			return
		}
		defer rc.Close()

		dlName := ""
		if len(segments) > 1 {
			dlName = segments[1]
		}
		if dlName == "" {
			safeTitle := bookutil.SanitizeFilename(title)
			safeAuthor := bookutil.SanitizeFilename(author)
			if safeTitle == "" {
				safeTitle = fileName
			}
			if safeAuthor != "" {
				dlName = fmt.Sprintf("%s - %s.%s", safeTitle, safeAuthor, format)
			} else {
				dlName = fmt.Sprintf("%s.%s", safeTitle, format)
			}
		}

		w.Header().Set("Content-Type", bookutil.MimeForFormat(format))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, bookutil.SanitizeFilename(dlName)))
		_, _ = io.Copy(w, rc)
	}
}

func configHandler(host Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg, err := config.Load()
			if err != nil {
				http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"books_dir":                cfg.BooksDir,
				"port":                     cfg.Port,
				"opds_root":                cfg.OPDSRoot,
				"reader_enabled":           cfg.ReaderEnabled,
				"reader_url":               cfg.ReaderURL,
				"default_search_language":  cfg.DefaultSearchLanguage,
				"public_base_url":          cfg.PublicBaseURL,
			})
		case http.MethodPost:
			var req domain.ConfigRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Некорректный запрос", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.BooksDir) == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Поле «Папка с книгами» обязательно"})
				return
			}
			cfg, err := config.Load()
			if err != nil {
				http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
				return
			}
			cfg.BooksDir = strings.TrimSpace(req.BooksDir)
			if req.Port != "" {
				cfg.Port = strings.TrimSpace(req.Port)
			}
			cfg.ReaderEnabled = req.ReaderEnabled
			if req.ReaderURL != "" {
				cfg.ReaderURL = req.ReaderURL
			}
			cfg.DefaultSearchLanguage = strings.TrimSpace(req.DefaultSearchLanguage)
			if req.PublicBaseURL != "" {
				cfg.PublicBaseURL = strings.TrimSpace(req.PublicBaseURL)
			}
			if pw := strings.TrimSpace(req.WebPassword); pw != "" {
				hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
				if err != nil {
					http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
					return
				}
				cfg.WebPasswordHash = string(hash)
			}
			if err := config.Save(cfg); err != nil {
				http.Error(w, "Ошибка сохранения конфигурации", http.StatusInternalServerError)
				return
			}
			if err := host.ReloadServices(true); err != nil {
				log.Printf("config: reload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Конфигурация сохранена"})
		default:
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		}
	}
}

func readerConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"reader_enabled":          cfg.ReaderEnabled,
		"reader_url":              cfg.ReaderURL,
		"default_search_language": cfg.DefaultSearchLanguage,
	})
}

func webAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{
		"password_required": cfg.WebPasswordHash != "",
	})
}

func webAuthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, "Ошибка чтения конфигурации", http.StatusInternalServerError)
		return
	}
	if cfg.WebPasswordHash == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.WebPasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Неверный пароль", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     webAuthCookieName,
		Value:    "authenticated",
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
}
