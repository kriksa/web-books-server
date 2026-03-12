package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// handlers_opds.go — генерация OPDS (XML Atom) каталога.
//
// Задача OPDS:
// - дать “ридерам” стандартный API для поиска и скачивания книг;
// - выдавать ссылки на обложки и корректные MIME-типы.
//
// Здесь важно аккуратно собирать URL и “безопасные” имена файлов, чтобы:
// - не ломать скачивания на разных клиентах;
// - не допустить недопустимых символов в Content-Disposition.

func createAcquisitionEntry(book Book, baseURL string, seriesContext bool) OPDSEntry {
	fileFormat := ensureFormat(book.Format, book.FileName)
	safeTitle := sanitizeFilename(book.Title)
	safeAuthor := sanitizeFilename(book.Author)
	if safeTitle == "" {
		safeTitle = "book"
	}
	// Используем тире и избегаем двойных пробелов перед escap'ом
	urlFilename := fmt.Sprintf("%s - %s.%s", safeTitle, safeAuthor, fileFormat)
	urlFilename = strings.ReplaceAll(urlFilename, "  ", " ")
	urlFilename = url.PathEscape(urlFilename)

	downloadURL := fmt.Sprintf("%s/download/%d/%s",
				   baseURL,
			    book.ID,
			    urlFilename,
	)

	mime := mimeForFormat(fileFormat)
	coverURL := fmt.Sprintf("%s/api/cover?file=%s&format=%s&zip=%s",
				baseURL,
			 url.QueryEscape(book.FileName),
				url.QueryEscape(fileFormat),
				url.QueryEscape(book.Zip),
	)

	descriptionText := fmt.Sprintf("Автор: %s. Жанр: %s. Формат: %s", book.Author, book.Genre, fileFormat)
	if book.Series != "" {
		descriptionText = fmt.Sprintf("Автор: %s. Серия: %s (№%d). Жанр: %s. Формат: %s",
					      book.Author, book.Series, book.SeriesNo, book.Genre, fileFormat)
	}

	displayTitle := book.Title
	if seriesContext && book.Series != "" {
		if book.SeriesNo > 0 {
			displayTitle = "#" + fmt.Sprint(book.SeriesNo) + " " + book.Title
		} else {
			displayTitle = "— " + book.Title
		}
	}

	entry := OPDSEntry{
		ID:         fmt.Sprintf("urn:uuid:book-%d", book.ID),
		Title:      displayTitle,
		Updated:    book.AddedAt.Format(time.RFC3339),
		Author:     &OPDSAuthor{Name: book.Author},
		Content:    &OPDSText{Type: "text", Text: descriptionText},
		Language:   book.Language,
		Identifier: fmt.Sprintf("BookID:%d", book.ID),
		Links: []OPDSLink{
			{Rel: "http://opds-spec.org/acquisition", Href: downloadURL, Type: mime,
				Title: fmt.Sprintf("Скачать %s (%s)", book.Title, strings.ToUpper(fileFormat))},
				{Rel: "http://opds-spec.org/image/thumbnail", Href: coverURL, Type: "image/jpeg"},
				{Rel: "http://opds-spec.org/image", Href: coverURL, Type: "image/jpeg"},
		},
	}

	if book.Genre != "" {
		genres := strings.Split(book.Genre, ",")
		for _, genre := range genres {
			genre = strings.TrimSpace(genre)
			if genre != "" {
				entry.Category = append(entry.Category, OPDSCategory{
					Term:  genre,
					Label: genre,
				})
			}
		}
	}

	return entry
}

func opdsAcquisitionFeed(w http.ResponseWriter, r *http.Request, books []Book, baseTitle string, seriesContext bool) {
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	baseURL := "http://" + r.Host
	searchLinkOpenSearch := OPDSLink{
		Rel:  "search",
		Href: baseURL + "/opds/opensearch",
		Type: "application/opensearchdescription+xml",
	}
	searchLinkTemplate := OPDSLink{
		Rel:  "search",
		Href: baseURL + "/opds/search?term={searchTerms}",
		Type: "application/atom+xml",
	}

	feed := OPDSFeed{
		XMLNSOPDS: OPDSNS,
		XMLNSDC:   DCNs,
		ID:        fmt.Sprintf("urn:uuid:acquisition-feed-%s-%d", strings.ReplaceAll(baseTitle, " ", "_"), time.Now().Unix()),
		Title:     baseTitle,
		Updated:   time.Now().Format(time.RFC3339),
		Author:    &OPDSAuthor{Name: "Web Books Server"},
		Links: []OPDSLink{
			{Rel: "start", Href: baseURL + "/opds/", Type: "application/atom+xml;profile=opds-catalog"},
			{Rel: "self", Href: r.URL.String(), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"},
			searchLinkOpenSearch,
			searchLinkTemplate,
		},
	}

	for _, book := range books {
		feed.Entries = append(feed.Entries, createAcquisitionEntry(book, baseURL, seriesContext))
	}

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		log.Printf("Ошибка маршалинга XML: %v", err)
		http.Error(w, "Ошибка формирования XML", http.StatusInternalServerError)
		return
	}

	fullResponse := []byte(xml.Header)
	fullResponse = append(fullResponse, output...)

	if _, err := w.Write(fullResponse); err != nil {
		if strings.Contains(err.Error(), "broken pipe") {
			log.Printf("Клиент отключился во время отправки OPDS-фида")
		} else {
			log.Printf("Ошибка записи OPDS-фида: %v", err)
		}
	}
}

func opdsNavigationFeed(w http.ResponseWriter, r *http.Request, title string, entries []OPDSEntry) {
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog")
	baseURL := "http://" + r.Host
	searchLinkOpenSearch := OPDSLink{
		Rel:  "search",
		Href: baseURL + "/opds/opensearch",
		Type: "application/opensearchdescription+xml",
	}
	searchLinkTemplate := OPDSLink{
		Rel:  "search",
		Href: baseURL + "/opds/search?term={searchTerms}",
		Type: "application/atom+xml",
	}

	feed := OPDSFeed{
		XMLNSOPDS: OPDSNS,
		XMLNSDC:   DCNs,
		ID:        fmt.Sprintf("urn:uuid:navigation-feed-%s-%d", strings.ReplaceAll(title, " ", "_"), time.Now().Unix()),
		Title:     title,
		Updated:   time.Now().Format(time.RFC3339),
		Author:    &OPDSAuthor{Name: "Web Books Server"},
		Links: []OPDSLink{
			{Rel: "start", Href: baseURL + "/opds/", Type: "application/atom+xml;profile=opds-catalog"},
			{Rel: "self", Href: r.URL.String(), Type: "application/atom+xml;profile=opds-catalog"},
			searchLinkOpenSearch,
			searchLinkTemplate,
		},
		Entries: entries,
	}

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		log.Printf("Ошибка маршалинга XML: %v", err)
		http.Error(w, "Ошибка формирования XML", http.StatusInternalServerError)
		return
	}

	fullResponse := []byte(xml.Header)
	fullResponse = append(fullResponse, output...)

	if _, err := w.Write(fullResponse); err != nil {
		if strings.Contains(err.Error(), "broken pipe") {
			log.Printf("Клиент отключился во время отправки OPDS-фида")
		} else {
			log.Printf("Ошибка записи OPDS-фида: %v", err)
		}
	}
}

func opdsRootHandler(w http.ResponseWriter, r *http.Request) {
	baseURL := "http://" + r.Host
	entries := []OPDSEntry{
		{
			ID:      "urn:uuid:authors",
			Title:   "Авторы",
			Updated: time.Now().Format(time.RFC3339),
			Content: &OPDSText{Type: "text", Text: "Список авторов."},
			Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/authors", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:titles",
			Title:   "Названия",
			Updated: time.Now().Format(time.RFC3339),
			Content: &OPDSText{Type: "text", Text: "Список книг по названию."},
			Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/titles", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:series",
			Title:   "Серии",
			Updated: time.Now().Format(time.RFC3339),
			Content: &OPDSText{Type: "text", Text: "Список серий."},
			Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/series", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:new",
			Title:   "Новые поступления",
			Updated: time.Now().Format(time.RFC3339),
			Content: &OPDSText{Type: "text", Text: "Последние добавленные книги."},
			Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/new", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
		},
	}

	opdsNavigationFeed(w, r, "Корневой OPDS-каталог Web Books Server", entries)
}

func opdsNewHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		books, err := dm.GetLatestBooks(100)
		if err != nil {
			log.Printf("Ошибка получения новых книг: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}
		opdsAcquisitionFeed(w, r, books, "Новые поступления", false)
	}
}

func opdsAuthorsHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			prefix = "А"
		}
		authors, err := dm.GetAuthors(prefix)
		if err != nil {
			log.Printf("Ошибка получения авторов: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		baseURL := "http://" + r.Host
		var entries []OPDSEntry

		for _, author := range authors {
			// Получаем количество книг автора
			filters := SearchFilters{Author: author}
			_, total, _ := dm.AdvancedSearchBooks(filters, 1, 1)

			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:author-%s", url.PathEscape(author)),
					 Title:   fmt.Sprintf("%s (%d)", author, total),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/author/" + url.PathEscape(author), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
			})
		}

		alphabet := "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯABCDEFGHIJKLMNOPQRSTUVWXYZ"
		for _, letter := range alphabet {
			letterStr := string(letter)
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:author-prefix-%s", letterStr),
					 Title:   fmt.Sprintf("Авторы на '%s'", letterStr),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/authors?prefix=" + url.QueryEscape(letterStr), Type: "application/atom+xml;profile=opds-catalog"}},
			})
		}

		opdsNavigationFeed(w, r, "Авторы", entries)
	}
}

func opdsAuthorHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		author := strings.TrimPrefix(r.URL.Path, "/opds/author/")
		author, _ = url.PathUnescape(author)
		filters := SearchFilters{Author: author}
		books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг автора: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Группируем по сериям
		seriesMap := make(map[string][]Book)
		var standaloneBooks []Book

		for _, book := range books {
			if book.Series != "" {
				seriesMap[book.Series] = append(seriesMap[book.Series], book)
			} else {
				standaloneBooks = append(standaloneBooks, book)
			}
		}

		baseURL := "http://" + r.Host
		var entries []OPDSEntry

		// Сначала показываем серии
		for seriesName, seriesBooks := range seriesMap {
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:author-series-%s-%s", url.PathEscape(author), url.PathEscape(seriesName)),
					 Title:   fmt.Sprintf("Серия: %s (%d книг)", seriesName, len(seriesBooks)),
					 Updated: time.Now().Format(time.RFC3339),
					 Links: []OPDSLink{{
						 Rel:  "subsection",
						 Href: baseURL + "/opds/serie/" + url.PathEscape(seriesName) + "?author=" + url.PathEscape(author),
					 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
					 }},
			})
		}

		// Потом отдельные книги
		if len(standaloneBooks) > 0 {
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:author-standalone-%s", url.PathEscape(author)),
					 Title:   fmt.Sprintf("Отдельные книги (%d)", len(standaloneBooks)),
					 Updated: time.Now().Format(time.RFC3339),
					 Links: []OPDSLink{{
						 Rel:  "subsection",
						 Href: baseURL + "/opds/author-standalone/" + url.PathEscape(author),
					 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
					 }},
			})
		}

		title := fmt.Sprintf("Автор: %s (всего книг: %d)", author, total)
		opdsNavigationFeed(w, r, title, entries)
	}
}

func opdsAuthorStandaloneHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		author := strings.TrimPrefix(r.URL.Path, "/opds/author-standalone/")
		author, _ = url.PathUnescape(author)
		filters := SearchFilters{Author: author}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг автора: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Оставляем только книги без серий
		var standalone []Book
		for _, book := range books {
			if book.Series == "" {
				standalone = append(standalone, book)
			}
		}

		opdsAcquisitionFeed(w, r, standalone, fmt.Sprintf("Отдельные книги автора: %s", author), false)
	}
}

func opdsTitlesHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			prefix = "А"
		}
		titles, err := dm.GetTitles(prefix)
		if err != nil {
			log.Printf("Ошибка получения названий: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		baseURL := "http://" + r.Host
		var entries []OPDSEntry

		for _, title := range titles {
			filters := SearchFilters{Title: title}
			_, total, _ := dm.AdvancedSearchBooks(filters, 1, 1)

			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:title-%s", url.PathEscape(title)),
					 Title:   fmt.Sprintf("%s (%d)", title, total),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/title/" + url.PathEscape(title), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
			})
		}

		alphabet := "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯABCDEFGHIJKLMNOPQRSTUVWXYZ"
		for _, letter := range alphabet {
			letterStr := string(letter)
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:title-prefix-%s", letterStr),
					 Title:   fmt.Sprintf("Названия на '%s'", letterStr),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/titles?prefix=" + url.QueryEscape(letterStr), Type: "application/atom+xml;profile=opds-catalog"}},
			})
		}

		opdsNavigationFeed(w, r, "Названия", entries)
	}
}

func opdsTitleHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title/")
		title, _ = url.PathUnescape(title)
		filters := SearchFilters{Title: title}
		books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг по названию: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Группируем по сериям
		seriesMap := make(map[string][]Book)
		var exactTitleBooks []Book

		for _, book := range books {
			if book.Series != "" {
				seriesMap[book.Series] = append(seriesMap[book.Series], book)
			} else {
				exactTitleBooks = append(exactTitleBooks, book)
			}
		}

		baseURL := "http://" + r.Host
		var entries []OPDSEntry

		// Серии, содержащие книги с таким названием
		for seriesName, seriesBooks := range seriesMap {
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:title-series-%s-%s", url.PathEscape(title), url.PathEscape(seriesName)),
					 Title:   fmt.Sprintf("В серии: %s (%d книг)", seriesName, len(seriesBooks)),
					 Updated: time.Now().Format(time.RFC3339),
					 Links: []OPDSLink{{
						 Rel:  "subsection",
						 Href: baseURL + "/opds/title-in-series/" + url.PathEscape(title) + "?series=" + url.PathEscape(seriesName),
					 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
					 }},
			})
		}

		// Отдельные книги с точным названием
		if len(exactTitleBooks) > 0 {
			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:title-exact-%s", url.PathEscape(title)),
					 Title:   fmt.Sprintf("Книги с названием '%s' (%d)", title, len(exactTitleBooks)),
					 Updated: time.Now().Format(time.RFC3339),
					 Links: []OPDSLink{{
						 Rel:  "subsection",
						 Href: baseURL + "/opds/title-exact/" + url.PathEscape(title),
					 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
					 }},
			})
		}

		opdsNavigationFeed(w, r, fmt.Sprintf("Название: %s (всего книг: %d)", title, total), entries)
	}
}

func opdsTitleExactHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title-exact/")
		title, _ = url.PathUnescape(title)
		filters := SearchFilters{Title: title}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 100)
		if err != nil {
			log.Printf("Ошибка получения книг: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Оставляем только книги без серий
		var exactBooks []Book
		for _, book := range books {
			if book.Series == "" {
				exactBooks = append(exactBooks, book)
			}
		}

		opdsAcquisitionFeed(w, r, exactBooks, fmt.Sprintf("Книги с названием '%s'", title), false)
	}
}

func opdsTitleInSeriesHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title-in-series/")
		title, _ = url.PathUnescape(title)
		seriesName := r.URL.Query().Get("series")
		filters := SearchFilters{Title: title}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 100)
		if err != nil {
			log.Printf("Ошибка получения книг: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Оставляем только книги в указанной серии
		var seriesBooks []Book
		for _, book := range books {
			if book.Series == seriesName {
				seriesBooks = append(seriesBooks, book)
			}
		}

		opdsAcquisitionFeed(w, r, seriesBooks, fmt.Sprintf("Книги '%s' в серии '%s'", title, seriesName), true)
	}
}

func opdsSeriesHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series, err := dm.GetSeries()
		if err != nil {
			log.Printf("Ошибка получения серий: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}
		baseURL := "http://" + r.Host
		var entries []OPDSEntry

		for _, s := range series {
			filters := SearchFilters{Series: s}
			_, total, _ := dm.AdvancedSearchBooks(filters, 1, 1)

			entries = append(entries, OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:series-%s", url.PathEscape(s)),
					 Title:   fmt.Sprintf("%s (%d)", s, total),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/serie/" + url.PathEscape(s), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
			})
		}

		opdsNavigationFeed(w, r, "Серии", entries)
	}
}

func opdsSerieHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series := strings.TrimPrefix(r.URL.Path, "/opds/serie/")
		series, _ = url.PathUnescape(series)
		filters := SearchFilters{Series: series}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг серии: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Сортируем книги в серии по номеру
		sort.Slice(books, func(i, j int) bool {
			if books[i].SeriesNo != books[j].SeriesNo {
				return books[i].SeriesNo < books[j].SeriesNo
			}
			return books[i].Title < books[j].Title
		})

		opdsAcquisitionFeed(w, r, books, "Книги серии: "+series, true)
	}
}

func opdsSearchHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("term")
		baseURL := "http://" + r.Host
		if query == "" {
			entries := []OPDSEntry{
				{
					ID:      "urn:uuid:search-form",
					Title:   "Поиск книг",
					Updated: time.Now().Format(time.RFC3339),
					Content: &OPDSText{Type: "text", Text: "Введите поисковый запрос."},
					Links:   []OPDSLink{{Rel: "search", Href: baseURL + "/opds/search?term={searchTerms}", Type: "application/atom+xml;profile=opds-catalog"}},
				},
				{
					ID:      "urn:uuid:search-tips",
					Title:   "Советы по поиску",
					Updated: time.Now().Format(time.RFC3339),
					Content: &OPDSText{Type: "text", Text: "Можно искать по автору, названию или серии."},
				},
			}
			opdsNavigationFeed(w, r, "Поиск", entries)
			return
		}

		queryEsc := url.QueryEscape(query)

		// Получаем количество результатов для каждого типа
		filters := SearchFilters{Author: query}
		_, authorTotal, _ := dm.AdvancedSearchBooks(filters, 1, 1)

		filters = SearchFilters{Title: query}
		_, titleTotal, _ := dm.AdvancedSearchBooks(filters, 1, 1)

		filters = SearchFilters{Series: query}
		_, seriesTotal, _ := dm.AdvancedSearchBooks(filters, 1, 1)

		entries := []OPDSEntry{
			{
				ID:      fmt.Sprintf("urn:uuid:search-authors-%s", queryEsc),
				Title:   fmt.Sprintf("Авторы: \"%s\" (%d)", query, authorTotal),
				Updated: time.Now().Format(time.RFC3339),
				Links: []OPDSLink{{
					Rel:  "subsection",
					Href: fmt.Sprintf("%s/opds/search-results?type=author&term=%s", baseURL, queryEsc),
					Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
				}},
			},
			{
				ID:      fmt.Sprintf("urn:uuid:search-titles-%s", queryEsc),
				Title:   fmt.Sprintf("Названия: \"%s\" (%d)", query, titleTotal),
				Updated: time.Now().Format(time.RFC3339),
				Links: []OPDSLink{{
					Rel:  "subsection",
					Href: fmt.Sprintf("%s/opds/search-results?type=title&term=%s", baseURL, queryEsc),
					Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
				}},
			},
			{
				ID:      fmt.Sprintf("urn:uuid:search-series-%s", queryEsc),
				Title:   fmt.Sprintf("Серии: \"%s\" (%d)", query, seriesTotal),
				Updated: time.Now().Format(time.RFC3339),
				Links: []OPDSLink{{
					Rel:  "subsection",
					Href: fmt.Sprintf("%s/opds/search-results?type=series&term=%s", baseURL, queryEsc),
					Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
				}},
			},
		}
		opdsNavigationFeed(w, r, fmt.Sprintf("Где искать: %s", query), entries)
	}
}

func opdsSearchResultsHandler(dm *DBManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searchType := r.URL.Query().Get("type")
		searchTerm := r.URL.Query().Get("term")
		if searchType == "" || searchTerm == "" {
			http.Error(w, "Отсутствуют параметры type или term", http.StatusBadRequest)
			return
		}

		baseURL := "http://" + r.Host
		var entries []OPDSEntry
		var title string

		switch searchType {
			case "author":
				filters := SearchFilters{Author: searchTerm}
				books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
				if err != nil {
					log.Printf("Ошибка поиска по автору: %v", err)
					http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
					return
				}

				// Группируем по сериям
				seriesMap := make(map[string][]Book)
				var standaloneBooks []Book

				for _, book := range books {
					if book.Series != "" {
						seriesMap[book.Series] = append(seriesMap[book.Series], book)
					} else {
						standaloneBooks = append(standaloneBooks, book)
					}
				}

				// Сначала показываем серии
				for seriesName, seriesBooks := range seriesMap {
					entries = append(entries, OPDSEntry{
						ID:      fmt.Sprintf("urn:uuid:search-author-series-%s-%s", url.PathEscape(searchTerm), url.PathEscape(seriesName)),
							 Title:   fmt.Sprintf("Серия: %s (%d книг)", seriesName, len(seriesBooks)),
							 Updated: time.Now().Format(time.RFC3339),
							 Links: []OPDSLink{{
								 Rel:  "subsection",
								 Href: baseURL + "/opds/serie/" + url.PathEscape(seriesName) + "?author=" + url.PathEscape(searchTerm),
							 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
							 }},
					})
				}

				// Потом отдельные книги
				if len(standaloneBooks) > 0 {
					entries = append(entries, OPDSEntry{
						ID:      fmt.Sprintf("urn:uuid:search-author-standalone-%s", url.PathEscape(searchTerm)),
							 Title:   fmt.Sprintf("Отдельные книги (%d)", len(standaloneBooks)),
							 Updated: time.Now().Format(time.RFC3339),
							 Links: []OPDSLink{{
								 Rel:  "subsection",
								 Href: baseURL + "/opds/author-standalone/" + url.PathEscape(searchTerm),
							 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
							 }},
					})
				}

				title = fmt.Sprintf("Поиск по автору: %s (найдено %d книг)", searchTerm, total)

				case "title":
					filters := SearchFilters{Title: searchTerm}
					books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
					if err != nil {
						log.Printf("Ошибка поиска по названию: %v", err)
						http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
						return
					}

					// Группируем по сериям
					seriesMap := make(map[string][]Book)
					var exactTitleBooks []Book

					for _, book := range books {
						if book.Series != "" {
							seriesMap[book.Series] = append(seriesMap[book.Series], book)
						} else {
							exactTitleBooks = append(exactTitleBooks, book)
						}
					}

					// Серии, содержащие книги с таким названием
					for seriesName, seriesBooks := range seriesMap {
						entries = append(entries, OPDSEntry{
							ID:      fmt.Sprintf("urn:uuid:search-title-series-%s-%s", url.PathEscape(searchTerm), url.PathEscape(seriesName)),
								 Title:   fmt.Sprintf("В серии: %s (%d книг)", seriesName, len(seriesBooks)),
								 Updated: time.Now().Format(time.RFC3339),
								 Links: []OPDSLink{{
									 Rel:  "subsection",
									 Href: baseURL + "/opds/title-in-series/" + url.PathEscape(searchTerm) + "?series=" + url.PathEscape(seriesName),
								 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
								 }},
						})
					}

					// Отдельные книги с точным названием
					if len(exactTitleBooks) > 0 {
						entries = append(entries, OPDSEntry{
							ID:      fmt.Sprintf("urn:uuid:search-title-exact-%s", url.PathEscape(searchTerm)),
								 Title:   fmt.Sprintf("Книги с названием '%s' (%d)", searchTerm, len(exactTitleBooks)),
								 Updated: time.Now().Format(time.RFC3339),
								 Links: []OPDSLink{{
									 Rel:  "subsection",
									 Href: baseURL + "/opds/title-exact/" + url.PathEscape(searchTerm),
								 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
								 }},
						})
					}

					title = fmt.Sprintf("Поиск по названию: %s (найдено %d книг)", searchTerm, total)

					case "series":
						// Получаем все книги, соответствующие поиску по сериям
						filters := SearchFilters{Series: searchTerm}
						books, _, err := dm.AdvancedSearchBooks(filters, 1, 10000)
						if err != nil {
							log.Printf("Ошибка поиска по сериям: %v", err)
							http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
							return
						}

						// Группируем книги по сериям
						seriesMap := make(map[string][]Book)
						for _, book := range books {
							if book.Series != "" {
								seriesMap[book.Series] = append(seriesMap[book.Series], book)
							}
						}

						// Преобразуем map в slice для сортировки
						type seriesInfo struct {
							Name  string
							Books []Book
						}
						var seriesList []seriesInfo
						for name, books := range seriesMap {
							seriesList = append(seriesList, seriesInfo{Name: name, Books: books})
						}

						// Сортируем серии по релевантности
						sort.Slice(seriesList, func(i, j int) bool {
							pi := getTitlePriority(seriesList[i].Name, searchTerm)
							pj := getTitlePriority(seriesList[j].Name, searchTerm)

							if pi != pj {
								return pi < pj
							}
							return strings.ToLower(seriesList[i].Name) < strings.ToLower(seriesList[j].Name)
						})

						// Создаем навигационные записи для каждой найденной серии
						for _, series := range seriesList {
							entries = append(entries, OPDSEntry{
								ID:      fmt.Sprintf("urn:uuid:search-series-%s", url.PathEscape(series.Name)),
									 Title:   fmt.Sprintf("%s (%d книг)", series.Name, len(series.Books)),
									 Updated: time.Now().Format(time.RFC3339),
									 Links: []OPDSLink{{
										 Rel:  "subsection",
										 Href: baseURL + "/opds/serie/" + url.PathEscape(series.Name),
									 Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
									 }},
							})
						}

						title = fmt.Sprintf("Найденные серии по запросу: %s (найдено %d серий)", searchTerm, len(entries))
		}

		opdsNavigationFeed(w, r, title, entries)
	}
}

func opdsOpenSearchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/opensearchdescription+xml")
		const opdsRoot = "/opds"
		searchURLTemplate := fmt.Sprintf("%s/search?term={searchTerms}", opdsRoot)
		desc := OpenSearchDescription{
			XMLNS:       "http://a9.com/-/spec/opensearch/1.1/",
			ShortName:   "inpx-web",
			Description: "Поиск по каталогу",
			InputEncoding:  "UTF-8",
			OutputEncoding: "UTF-8",
			URL: OpenSearchURL{
				Type:     "application/atom+xml;profile=opds-catalog;kind=navigation",
				Template: searchURLTemplate,
			},
		}

		output, err := xml.MarshalIndent(desc, "", "  ")
		if err != nil {
			log.Printf("Ошибка маршалинга OpenSearch XML: %v", err)
			http.Error(w, "Ошибка формирования XML", http.StatusInternalServerError)
			return
		}

		fullResponse := []byte(xml.Header)
		fullResponse = append(fullResponse, output...)
		w.Write(fullResponse)
	}
}
