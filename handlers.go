package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/timsims/pamphlet"
	"golang.org/x/crypto/bcrypt"
)

// readerConfigHandler возвращает только настройки читалки (без авторизации).
func readerConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("Ошибка загрузки конфига (reader): %v", err)
		http.Error(w, "Не удалось загрузить конфигурацию", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reader_enabled":          cfg.ReaderEnabled,
		"reader_url":              cfg.ReaderURL,
		"default_search_language": cfg.DefaultSearchLanguage,
	})
}

func configHandler(sm *SystemManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		switch r.Method {
		case http.MethodGet:
			cfg, err := LoadConfig()
			if err != nil {
				log.Printf("Ошибка загрузки конфига в API: %v", err)
				http.Error(w, "Не удалось загрузить конфигурацию", http.StatusInternalServerError)
				return
			}

			// ИСПРАВЛЕНИЕ: Правильный формат ответа для фронтенда
			response := map[string]interface{}{
				"books_dir":                cfg.BooksDir,
				"port":                     cfg.Port,
				"opds_root":                cfg.OPDSRoot,
				"web_password":             "", // Не возвращаем пароль при GET
				"reader_enabled":           cfg.ReaderEnabled,
				"reader_url":               cfg.ReaderURL,
				"default_search_language":  cfg.DefaultSearchLanguage,
			}

			json.NewEncoder(w).Encode(response)

		case http.MethodPost:
			// ИСПРАВЛЕНИЕ: Правильная структура запроса
			var req struct {
				BooksDir               string `json:"books_dir"`
				Port                   string `json:"port"`
				OPDSRoot               string `json:"opds_root"`
				WebPassword            string `json:"web_password"`
				ReaderEnabled          bool   `json:"reader_enabled"`
				ReaderURL              string `json:"reader_url"`
				DefaultSearchLanguage  string `json:"default_search_language"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Некорректный JSON: " + err.Error(), http.StatusBadRequest)
				return
			}

			if req.BooksDir == "" {
				http.Error(w, "Обязательные поля не заполнены", http.StatusBadRequest)
				return
			}

			currentCfg, err := LoadConfig()
			if err != nil {
				http.Error(w, "Ошибка чтения файла конфигурации", http.StatusInternalServerError)
				return
			}

			needRestart := currentCfg.BooksDir != req.BooksDir

			currentCfg.BooksDir = req.BooksDir
			currentCfg.Port = req.Port
			currentCfg.OPDSRoot = req.OPDSRoot
			currentCfg.ReaderEnabled = req.ReaderEnabled
			currentCfg.ReaderURL = req.ReaderURL
			currentCfg.DefaultSearchLanguage = strings.TrimSpace(req.DefaultSearchLanguage)

			// ИСПРАВЛЕНИЕ: Обрабатываем удаление пароля
			if req.WebPassword == "" {
				// Если пришёл пустой пароль - удаляем существующий
				currentCfg.WebPasswordHash = ""
			} else {
				// Если пароль не пустой - хешируем и сохраняем новый
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.WebPassword), bcrypt.DefaultCost)
				if err != nil {
					http.Error(w, "Ошибка хеширования пароля", http.StatusInternalServerError)
					return
				}
				currentCfg.WebPasswordHash = string(hashedPassword)
			}

			if err := SaveConfig(currentCfg); err != nil {
				log.Printf("Ошибка сохранения конфига: %v", err)
				http.Error(w, "Ошибка записи файла", http.StatusInternalServerError)
				return
			}

			if needRestart {
				go sm.ReloadServices(true)
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":    "ok",
				"restarting": needRestart,
			})

		default:
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
		}
	}
}

func webAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := LoadConfig()
	if err != nil {
		http.Error(w, "Ошибка конфигурации", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	required := cfg.WebPasswordHash != ""

	if required {
		cookie, err := r.Cookie("web_auth_session")
		if err == nil && cookie.Value == "authenticated" {
			required = false
		}
	}

	json.NewEncoder(w).Encode(map[string]bool{"password_required": required})
}

func webAuthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		http.Error(w, "Ошибка конфигурации", http.StatusInternalServerError)
		return
	}

	if cfg.WebPasswordHash == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cfg.WebPasswordHash), []byte(req.Password)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "Неверный пароль"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "web_auth_session",
		Value:    "authenticated",
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func apiBookDetailsHandler(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "ID is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		var fileName, zipName, format string
		var del int
		err = dm.db.QueryRow(`SELECT file_name, zip, format, del FROM books WHERE id = ?`, id).Scan(&fileName, &zipName, &format, &del)

		if err == sql.ErrNoRows {
			http.Error(w, "Book not found", http.StatusNotFound)
			return
		}
		if del == 1 {
			http.Error(w, "Book deleted", http.StatusNotFound)
			return
		}

		format = ensureFormat(format, fileName)
		if format != "fb2" && format != "epub" {
			http.Error(w, "Details available only for FB2 and EPUB files", http.StatusNotImplemented)
			return
		}

		zipPath := filepath.Join(booksDir, zipName)
		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			log.Printf("Archive not found: %s", zipPath)
			http.Error(w, "Archive not found", http.StatusNotFound)
			return
		}
		defer zf.Close()

		var targetFile *zip.File
		targetPattern := fmt.Sprintf("%s.%s", fileName, format)

		for _, f := range zf.File {
			if strings.EqualFold(f.Name, targetPattern) {
				targetFile = f
				break
			}
		}

		if targetFile == nil {
			http.Error(w, "File not found in archive", http.StatusNotFound)
			return
		}

		rc, err := targetFile.Open()
		if err != nil {
			http.Error(w, "Error opening file", http.StatusInternalServerError)
			return
		}
		defer rc.Close()

		content, err := io.ReadAll(rc)
		if err != nil {
			http.Error(w, "Error reading file content", http.StatusInternalServerError)
			return
		}

		var details DetailedBookInfo

		if format == "fb2" {
			fb2, err := ParseFB2Metadata(content)
			if err != nil {
				log.Printf("FB2 Parse Error ID %d: %v", id, err)
				http.Error(w, "Error parsing FB2 XML", http.StatusInternalServerError)
				return
			}
			details = ExtractDetailedInfo(fb2)
		} else if format == "epub" {
			parser, err := pamphlet.OpenBytes(content)
			if err != nil {
				log.Printf("EPUB Parse Error ID %d (Pamphlet): %v", id, err)
				http.Error(w, "Error parsing EPUB metadata", http.StatusInternalServerError)
				return
			}
			epubBook := parser.GetBook()
			details = ConvertPamphletToDetails(epubBook)
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(details)
	}
}

func apiSearchHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		pageStr := r.URL.Query().Get("page")
		limitStr := r.URL.Query().Get("limit")

		page := 1
		if val, err := strconv.Atoi(pageStr); err == nil && val > 0 {
			page = val
		}

		limit := 5000
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}

		filters := SearchFilters{
			Author:   strings.TrimSpace(r.URL.Query().Get("author")),
			Title:    strings.TrimSpace(r.URL.Query().Get("title")),
			Series:   strings.TrimSpace(r.URL.Query().Get("series")),
			Genre:    strings.TrimSpace(r.URL.Query().Get("genre")),
			Language: strings.TrimSpace(r.URL.Query().Get("language")),
		}

		books, total, err := dm.AdvancedSearchBooks(filters, page, limit)
		if err != nil {
			log.Printf("Ошибка поиска: %v", err)
			http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
			return
		}

		languages, err := dm.GetAvailableLanguages()
		if err != nil {
			languages = []string{}
		}

		response := struct {
			Books     []Book   `json:"books"`
			Total     int      `json:"total"`
			Page      int      `json:"page"`
			PerPage   int      `json:"perPage"`
			Languages []string `json:"languages"`
		}{
			Books:     books,
			Total:     total,
			Page:      page,
			PerPage:   limit,
			Languages: append([]string{"Все языки"}, languages...),
		}

		json.NewEncoder(w).Encode(response)
	}
}

func apiCoverHandler(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileName := r.URL.Query().Get("file")
		format := r.URL.Query().Get("format")
		zipName := r.URL.Query().Get("zip")

		if fileName == "" || format == "" || zipName == "" {
			http.Error(w, "Отсутствуют обязательные параметры", http.StatusBadRequest)
			return
		}

		if format != "fb2" && format != "epub" {
			http.Error(w, "Cover not found", http.StatusNotFound)
			return
		}

		cacheKey := fmt.Sprintf("%s:%s:%s", zipName, fileName, format)

		if data, ok := imageCache.Get(cacheKey); ok {
			if bytes.Equal(data, missingCoverMarker) {
				http.Error(w, "Cover not found", http.StatusNotFound)
			} else {
				w.Header().Set("Content-Type", "image/jpeg")
				w.Header().Set("Cache-Control", "public, max-age=3600")
				w.Write(data)
			}
			return
		}

		zipPath := filepath.Join(booksDir, zipName)
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			http.Error(w, "Cover not found", http.StatusNotFound)
			return
		}

		type coverResult struct {
			data []byte
			mime string
			err  error
		}
		resultChan := make(chan coverResult, 1)

		ctx, cancel := context.WithTimeout(context.Background(), CoverExtractTimeout)
		defer cancel()

		go func() {
			data, mime, err := extractCover(zipPath, fileName, format)
			resultChan <- coverResult{data: data, mime: mime, err: err}
		}()

		select {
		case <-ctx.Done():
			log.Printf("Таймаут извлечения обложки для: %s", cacheKey)
			imageCache.Add(cacheKey, missingCoverMarker)
			http.Error(w, "Cover not found", http.StatusNotFound)

		case res := <-resultChan:
			if res.err != nil {
				imageCache.Add(cacheKey, missingCoverMarker)
				http.Error(w, "Cover not found", http.StatusNotFound)
			} else {
				imageCache.Add(cacheKey, res.data)
				w.Header().Set("Content-Type", res.mime)
				w.Header().Set("Cache-Control", "public, max-age=3600")
				w.Write(res.data)
			}
		}
	}
}

func downloadHandler(dm *DBManager, booksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var bookID int

		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

		if len(pathParts) >= 2 {
			if id, err := strconv.Atoi(pathParts[1]); err == nil && id > 0 {
				bookID = id
			}
		}

		if bookID == 0 {
			http.Error(w, "Invalid Book ID", http.StatusBadRequest)
			return
		}

		// Используем функцию из DBManager для получения информации о книге
		fileName, zipName, format, title, language, author, del, err := dm.GetBookDownloadInfo(bookID)

		if err == sql.ErrNoRows {
			http.Error(w, "Книга не найдена в базе", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("Ошибка БД при скачивании (ID %d): %v", bookID, err)
			http.Error(w, "Ошибка базы данных", http.StatusInternalServerError)
			return
		}
		if del == 1 {
			http.Error(w, "Книга удалена", http.StatusNotFound)
			return
		}
		format = ensureFormat(format, fileName)

		zipPath := filepath.Join(booksDir, zipName)
		if _, err := os.Stat(zipPath); os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("ZIP-файл %s не найден", zipName), http.StatusNotFound)
			return
		}

		zf, err := zip.OpenReader(zipPath)
		if err != nil {
			http.Error(w, "Не удалось открыть ZIP-архив", http.StatusInternalServerError)
			return
		}
		defer zf.Close()

		var targetFile *zip.File
		targetName := fmt.Sprintf("%s.%s", fileName, format)

		for _, f := range zf.File {
			if strings.EqualFold(f.Name, targetName) {
				targetFile = f
				break
			}
		}

		if targetFile == nil {
			for _, f := range zf.File {
				if strings.EqualFold(f.Name, fileName) {
					targetFile = f
					break
				}
			}
		}

		if targetFile == nil {
			http.Error(w, fmt.Sprintf("Файл %s не найден в архиве", fileName), http.StatusNotFound)
			return
		}

		rc, err := targetFile.Open()
		if err != nil {
			http.Error(w, "Не удалось открыть файл в архиве", http.StatusInternalServerError)
			return
		}
		defer rc.Close()

		contentType := mimeForFormat(format)

		safeTitle := sanitizeFilename(title)
		safeAuthor := "unknown"
		if author != "" {
			safeAuthor = sanitizeFilename(author)
		}

		if safeTitle == "" {
			safeTitle = "book"
		}
		if language == "" {
			language = "unknown"
		}

		downloadFilename := fmt.Sprintf("%s - %s (%s).%s", safeTitle, safeAuthor, language, format)

		w.Header().Set("Content-Disposition", "attachment; filename=\""+downloadFilename+"\"")
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatUint(targetFile.UncompressedSize64, 10))

		bw := bufio.NewWriter(w)
		defer bw.Flush()

		if _, err := io.Copy(bw, rc); err != nil {
			if errors.Is(err, http.ErrHandlerTimeout) || errors.Is(err, http.ErrAbortHandler) {
				return
			}
		}
	}
}

func extractCoverFromFB2(content []byte) ([]byte, string, error) {
	fb2, err := ParseFB2Metadata(content)
	if err != nil {
		return nil, "", fmt.Errorf("ошибка парсинга FB2: %v", err)
	}

	img := fb2.Description.TitleInfo.Coverpage.Image
	imageHref := img.Href
	if imageHref == "" {
		imageHref = img.XLinkHref
	}
	if imageHref == "" {
		return nil, "", errCoverNotFound
	}

	if strings.HasPrefix(imageHref, "#") {
		imageID := strings.TrimPrefix(imageHref, "#")
		for _, binary := range fb2.Binary {
			if binary.Id == imageID {
				data, err := base64.StdEncoding.DecodeString(binary.Data)
				if err != nil {
					return nil, "", fmt.Errorf("ошибка декодирования обложки: %v", err)
				}
				mime := binary.ContentType
				if mime == "" {
					mime = "image/jpeg"
				}
				return data, mime, nil
			}
		}
	}
	return nil, "", errCoverNotFound
}

type ContainerXML struct {
	Rootfiles struct {
		Rootfile struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

type PackageOPF struct {
	Metadata struct {
		Meta []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Item []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			Media      string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
}

func extractCoverFromEPUB(content []byte) ([]byte, string, error) {
	r := bytes.NewReader(content)
	epubZip, err := zip.NewReader(r, r.Size())
	if err != nil {
		return nil, "", err
	}

	var opfPath string
	for _, f := range epubZip.File {
		if f.Name == "META-INF/container.xml" {
			rc, _ := f.Open()
			defer rc.Close()
			var container ContainerXML
			xml.NewDecoder(rc).Decode(&container)
			opfPath = container.Rootfiles.Rootfile.FullPath
			break
		}
	}
	if opfPath == "" {
		return nil, "", errCoverNotFound
	}

	var opfFile *zip.File
	for _, f := range epubZip.File {
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}
	if opfFile == nil {
		return nil, "", errCoverNotFound
	}

	rcOpf, _ := opfFile.Open()
	defer rcOpf.Close()
	var opf PackageOPF
	xml.NewDecoder(rcOpf).Decode(&opf)

	var coverID string
	for _, meta := range opf.Metadata.Meta {
		if meta.Name == "cover" {
			coverID = meta.Content
			break
		}
	}
	if coverID == "" {
		for _, item := range opf.Manifest.Item {
			if strings.HasPrefix(item.Media, "image/") &&
				(strings.Contains(strings.ToLower(item.ID), "cover") ||
					strings.Contains(strings.ToLower(item.Properties), "cover-image")) {
				coverID = item.ID
				break
			}
		}
	}

	if coverID == "" {
		return nil, "", errCoverNotFound
	}

	var coverHref, coverMime string
	for _, item := range opf.Manifest.Item {
		if item.ID == coverID {
			coverHref = item.Href
			coverMime = item.Media
			break
		}
	}
	if coverHref == "" {
		return nil, "", errCoverNotFound
	}

	coverPath := filepath.Join(filepath.Dir(opfPath), coverHref)
	coverPath = strings.ReplaceAll(coverPath, "\\", "/")
	if strings.HasPrefix(coverPath, "./") {
		coverPath = coverPath[2:]
	}
	coverBase := filepath.Base(coverPath)

	for _, f := range epubZip.File {
		if f.Name == coverPath {
			rc, _ := f.Open()
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return data, coverMime, nil
		}
	}
	// Некоторые EPUB используют разные пути к обложке — ищем по имени файла
	for _, f := range epubZip.File {
		if filepath.Base(f.Name) == coverBase {
			rc, _ := f.Open()
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return data, coverMime, nil
		}
	}
	return nil, "", errCoverNotFound
}

func extractCover(zipPath, fileName, format string) ([]byte, string, error) {
	zf, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer zf.Close()

	// Имя файла в архиве может быть "book.fb2" или "book" (без расширения); в БД — с расширением или без
	targetWithExt := fileName
	if format != "" && !strings.HasSuffix(strings.ToLower(fileName), "."+strings.ToLower(format)) {
		targetWithExt = fmt.Sprintf("%s.%s", fileName, format)
	}
	var targetFile *zip.File
	for _, f := range zf.File {
		if strings.EqualFold(f.Name, targetWithExt) || strings.EqualFold(f.Name, fileName) {
			targetFile = f
			break
		}
	}
	if targetFile == nil {
		// Файл может лежать в подпапке архива (например "Fiction/Book.fb2")
		baseTarget := filepath.Base(targetWithExt)
		baseFileName := filepath.Base(fileName)
		for _, f := range zf.File {
			base := filepath.Base(f.Name)
			if strings.EqualFold(base, baseTarget) || strings.EqualFold(base, baseFileName) {
				targetFile = f
				break
			}
		}
	}
	if targetFile == nil {
		return nil, "", errCoverNotFound
	}

	rc, err := targetFile.Open()
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", err
	}

	switch format {
	case "fb2":
		return extractCoverFromFB2(content)
	case "epub":
		return extractCoverFromEPUB(content)
	default:
		return nil, "", errCoverNotFound
	}
}
