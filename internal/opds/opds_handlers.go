package opds
import (
	"web_books/internal/bookutil"
	"web_books/internal/domain"
	"web_books/internal/storage"

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

func createAcquisitionEntry(book domain.Book, baseURL string, seriesContext bool) domain.OPDSEntry {
	fileFormat := bookutil.EnsureFormat(book.Format, book.FileName)
	safeTitle := bookutil.SanitizeFilename(book.Title)
	safeAuthor := bookutil.SanitizeFilename(book.Author)
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

	mime := bookutil.MimeForFormat(fileFormat)
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

	entry := domain.OPDSEntry{
		ID:         fmt.Sprintf("urn:uuid:book-%d", book.ID),
		Title:      displayTitle,
		Updated:    book.AddedAt.Format(time.RFC3339),
		Author:     &domain.OPDSAuthor{Name: book.Author},
		Content:    &domain.OPDSText{Type: "text", Text: descriptionText},
		Language:   book.Language,
		Identifier: fmt.Sprintf("BookID:%d", book.ID),
		Links: []domain.OPDSLink{
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
				entry.Category = append(entry.Category, domain.OPDSCategory{
					Term:  genre,
					Label: genre,
				})
			}
		}
	}

	return entry
}

func opdsAcquisitionFeed(w http.ResponseWriter, r *http.Request, books []domain.Book, baseTitle string, seriesContext bool) {
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	baseURL := opdsBaseURL(r)
	searchLink := opdsSearchLink(baseURL)

	feed := domain.OPDSFeed{
		XMLNSOPDS: domain.OPDSNS,
		XMLNSDC:   domain.DCNs,
		ID:        fmt.Sprintf("urn:uuid:acquisition-feed-%s-%d", strings.ReplaceAll(baseTitle, " ", "_"), time.Now().Unix()),
		Title:     baseTitle,
		Updated:   time.Now().Format(time.RFC3339),
		Author:    &domain.OPDSAuthor{Name: "Web Books Server"},
		Links: []domain.OPDSLink{
			{Rel: "start", Href: baseURL + "/opds/", Type: "application/atom+xml;profile=opds-catalog"},
			{Rel: "self", Href: opdsSelfURL(r), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"},
			searchLink,
		},
	}

	for _, book := range books {
		feed.Entries = append(feed.Entries, createAcquisitionEntry(book, baseURL, seriesContext))
	}

	writeXMLResponse(w, feed)
}

func opdsNavigationFeed(w http.ResponseWriter, r *http.Request, title string, entries []domain.OPDSEntry) {
	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog")
	baseURL := opdsBaseURL(r)
	searchLink := opdsSearchLink(baseURL)

	feed := domain.OPDSFeed{
		XMLNSOPDS: domain.OPDSNS,
		XMLNSDC:   domain.DCNs,
		ID:        fmt.Sprintf("urn:uuid:navigation-feed-%s-%d", strings.ReplaceAll(title, " ", "_"), time.Now().Unix()),
		Title:     title,
		Updated:   time.Now().Format(time.RFC3339),
		Author:    &domain.OPDSAuthor{Name: "Web Books Server"},
		Links: []domain.OPDSLink{
			{Rel: "start", Href: baseURL + "/opds/", Type: "application/atom+xml;profile=opds-catalog"},
			{Rel: "self", Href: opdsSelfURL(r), Type: "application/atom+xml;profile=opds-catalog"},
			searchLink,
		},
		Entries: entries,
	}

	writeXMLResponse(w, feed)
}

const opdsNavType = "application/atom+xml;profile=opds-catalog;kind=navigation"

// opdsSearchFormFeed is the dedicated search page for readers without global OpenSearch.
func opdsSearchFormFeed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", opdsNavType)
	baseURL := opdsBaseURL(r)
	searchLink := opdsSearchLink(baseURL)

	feed := domain.OPDSFeed{
		XMLNSOPDS: domain.OPDSNS,
		XMLNSDC:   domain.DCNs,
		ID:        "urn:uuid:search-form",
		Title:     "Поиск",
		Updated:   time.Now().Format(time.RFC3339),
		Author:    &domain.OPDSAuthor{Name: "Web Books Server"},
		Links: []domain.OPDSLink{
			{Rel: "start", Href: baseURL + "/opds/", Type: opdsNavType},
			{Rel: "self", Href: opdsSelfURL(r), Type: opdsNavType},
			searchLink,
		},
		Entries: []domain.OPDSEntry{
			{
				ID:      "urn:uuid:search-input",
				Title:   "Поиск",
				Updated: time.Now().Format(time.RFC3339),
				Links:   []domain.OPDSLink{searchLink},
			},
		},
	}

	writeXMLResponse(w, feed)
}

func opdsSearchChooserFeed(w http.ResponseWriter, r *http.Request, dm *storage.Manager, query string) {
	baseURL := opdsBaseURL(r)
	queryEsc := url.QueryEscape(query)

	authorTotal, _ := dm.CountMatchingBooks(domain.SearchFilters{Author: query})
	titleTotal, _ := dm.CountMatchingBooks(domain.SearchFilters{Title: query})
	seriesTotal, _ := dm.CountMatchingBooks(domain.SearchFilters{Series: query})

	entries := []domain.OPDSEntry{
		{
			ID:      fmt.Sprintf("urn:uuid:search-authors-%s", queryEsc),
			Title:   fmt.Sprintf("Авторы: \"%s\" (%d)", query, authorTotal),
			Updated: time.Now().Format(time.RFC3339),
			Links: []domain.OPDSLink{{
				Rel:  "subsection",
				Href: fmt.Sprintf("%s/opds/search-results?type=author&term=%s", baseURL, queryEsc),
				Type: "application/atom+xml;profile=opds-catalog",
			}},
		},
		{
			ID:      fmt.Sprintf("urn:uuid:search-titles-%s", queryEsc),
			Title:   fmt.Sprintf("Названия: \"%s\" (%d)", query, titleTotal),
			Updated: time.Now().Format(time.RFC3339),
			Links: []domain.OPDSLink{{
				Rel:  "subsection",
				Href: fmt.Sprintf("%s/opds/search-results?type=title&term=%s", baseURL, queryEsc),
				Type: "application/atom+xml;profile=opds-catalog",
			}},
		},
		{
			ID:      fmt.Sprintf("urn:uuid:search-series-%s", queryEsc),
			Title:   fmt.Sprintf("Серии: \"%s\" (%d)", query, seriesTotal),
			Updated: time.Now().Format(time.RFC3339),
			Links: []domain.OPDSLink{{
				Rel:  "subsection",
				Href: fmt.Sprintf("%s/opds/search-results?type=series&term=%s", baseURL, queryEsc),
				Type: "application/atom+xml;profile=opds-catalog",
			}},
		},
	}
	opdsNavigationFeed(w, r, fmt.Sprintf("Где искать: %s", query), entries)
}

func opdsRootHandler(cat *Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if path != "/opds" {
			http.NotFound(w, r)
			return
		}

		baseURL := opdsBaseURL(r)
		entries := []domain.OPDSEntry{
		{
			ID:      "urn:uuid:authors",
			Title:   "Авторы",
			Updated: time.Now().Format(time.RFC3339),
			Content: &domain.OPDSText{Type: "text", Text: "Список авторов."},
			Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/authors", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:titles",
			Title:   "Названия",
			Updated: time.Now().Format(time.RFC3339),
			Content: &domain.OPDSText{Type: "text", Text: "Список книг по названию."},
			Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/titles", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:series",
			Title:   "Серии",
			Updated: time.Now().Format(time.RFC3339),
			Content: &domain.OPDSText{Type: "text", Text: "Список серий."},
			Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/series", Type: "application/atom+xml;profile=opds-catalog"}},
		},
		{
			ID:      "urn:uuid:search",
			Title:   "Поиск",
			Updated: time.Now().Format(time.RFC3339),
			Content: &domain.OPDSText{Type: "text", Text: "Поиск по автору, названию или серии."},
			Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/search", Type: opdsNavType}},
		},
		{
			ID:      "urn:uuid:new",
			Title:   "Новые поступления",
			Updated: time.Now().Format(time.RFC3339),
			Content: &domain.OPDSText{Type: "text", Text: "Последние добавленные книги."},
			Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/new", Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
		},
	}

		opdsNavigationFeed(w, r, "Корневой OPDS-каталог Web Books Server", entries)
	}
}

func opdsNewHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
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

func opdsAuthorsHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			prefix = "А"
		}
		authors, err := dm.GetAuthorsWithBookCounts(prefix)
		if err != nil {
			log.Printf("Ошибка получения авторов: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		baseURL := opdsBaseURL(r)
		var entries []domain.OPDSEntry

		for _, author := range authors {
			entries = append(entries, domain.OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:author-%s", url.PathEscape(author.Name)),
					 Title:   fmt.Sprintf("%s (%d)", author.Name, author.Count),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/author/" + url.PathEscape(author.Name), Type: "application/atom+xml;profile=opds-catalog"}},
			})
		}

		entries = append(entries, opdsPrefixNavigationEntries(baseURL, "/opds/authors?prefix=", "author", "Авторы на '%s'")...)

		opdsNavigationFeed(w, r, "Авторы", entries)
	}
}

func opdsAuthorHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		author := strings.TrimPrefix(r.URL.Path, "/opds/author/")
		author, _ = url.PathUnescape(author)
		filters := domain.SearchFilters{Author: author}
		books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг автора: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		seriesMap, standaloneBooks := splitSeriesAndStandalone(books)

		baseURL := opdsBaseURL(r)
		entries := buildSeriesSubsectionEntries(seriesMap, seriesEntrySpec{
			IDPrefix:    fmt.Sprintf("author-series-%s", url.PathEscape(author)),
			TitlePrefix: "Серия: ",
			HrefFor: func(seriesName string) string {
				return baseURL + "/opds/serie/" + url.PathEscape(seriesName) + "?author=" + url.PathEscape(author)
			},
		})

		if len(standaloneBooks) > 0 {
			entries = append(entries, buildStandaloneSubsectionEntry(
				fmt.Sprintf("urn:uuid:author-standalone-%s", url.PathEscape(author)),
				fmt.Sprintf("Отдельные книги (%d)", len(standaloneBooks)),
				baseURL+"/opds/author-standalone/"+url.PathEscape(author),
			))
		}

		title := fmt.Sprintf("Автор: %s (всего книг: %d)", author, total)
		opdsNavigationFeed(w, r, title, entries)
	}
}

func opdsAuthorStandaloneHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		author := strings.TrimPrefix(r.URL.Path, "/opds/author-standalone/")
		author, _ = url.PathUnescape(author)
		filters := domain.SearchFilters{Author: author}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг автора: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		opdsAcquisitionFeed(w, r, booksWithoutSeries(books), fmt.Sprintf("Отдельные книги автора: %s", author), false)
	}
}

func opdsTitlesHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			prefix = "А"
		}
		titles, err := dm.GetTitlesWithBookCounts(prefix)
		if err != nil {
			log.Printf("Ошибка получения названий: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		baseURL := opdsBaseURL(r)
		var entries []domain.OPDSEntry

		for _, title := range titles {
			entries = append(entries, domain.OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:title-%s", url.PathEscape(title.Name)),
					 Title:   fmt.Sprintf("%s (%d)", title.Name, title.Count),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/title/" + url.PathEscape(title.Name), Type: "application/atom+xml;profile=opds-catalog"}},
			})
		}

		entries = append(entries, opdsPrefixNavigationEntries(baseURL, "/opds/titles?prefix=", "title", "Названия на '%s'")...)

		opdsNavigationFeed(w, r, "Названия", entries)
	}
}

func opdsTitleHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title/")
		title, _ = url.PathUnescape(title)
		filters := domain.SearchFilters{Title: title}
		books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
		if err != nil {
			log.Printf("Ошибка получения книг по названию: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		seriesMap, exactTitleBooks := splitSeriesAndStandalone(books)

		baseURL := opdsBaseURL(r)
		entries := buildSeriesSubsectionEntries(seriesMap, seriesEntrySpec{
			IDPrefix:    fmt.Sprintf("title-series-%s", url.PathEscape(title)),
			TitlePrefix: "В серии: ",
			HrefFor: func(seriesName string) string {
				return baseURL + "/opds/title-in-series/" + url.PathEscape(title) + "?series=" + url.PathEscape(seriesName)
			},
		})

		if len(exactTitleBooks) > 0 {
			entries = append(entries, buildStandaloneSubsectionEntry(
				fmt.Sprintf("urn:uuid:title-exact-%s", url.PathEscape(title)),
				fmt.Sprintf("Книги с названием '%s' (%d)", title, len(exactTitleBooks)),
				baseURL+"/opds/title-exact/"+url.PathEscape(title),
			))
		}

		opdsNavigationFeed(w, r, fmt.Sprintf("Название: %s (всего книг: %d)", title, total), entries)
	}
}

func opdsTitleExactHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title-exact/")
		title, _ = url.PathUnescape(title)
		filters := domain.SearchFilters{Title: title}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 100)
		if err != nil {
			log.Printf("Ошибка получения книг: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		opdsAcquisitionFeed(w, r, booksWithoutSeries(books), fmt.Sprintf("Книги с названием '%s'", title), false)
	}
}

func opdsTitleInSeriesHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := strings.TrimPrefix(r.URL.Path, "/opds/title-in-series/")
		title, _ = url.PathUnescape(title)
		seriesName := r.URL.Query().Get("series")
		filters := domain.SearchFilters{Title: title}
		books, _, err := dm.AdvancedSearchBooks(filters, 1, 100)
		if err != nil {
			log.Printf("Ошибка получения книг: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}

		// Оставляем только книги в указанной серии
		var seriesBooks []domain.Book
		for _, book := range books {
			if book.Series == seriesName {
				seriesBooks = append(seriesBooks, book)
			}
		}

		opdsAcquisitionFeed(w, r, seriesBooks, fmt.Sprintf("Книги '%s' в серии '%s'", title, seriesName), true)
	}
}

func opdsSeriesHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series, err := dm.GetSeriesWithBookCounts()
		if err != nil {
			log.Printf("Ошибка получения серий: %v", err)
			http.Error(w, "Ошибка получения данных", http.StatusInternalServerError)
			return
		}
		baseURL := opdsBaseURL(r)
		var entries []domain.OPDSEntry

		for _, s := range series {
			entries = append(entries, domain.OPDSEntry{
				ID:      fmt.Sprintf("urn:uuid:series-%s", url.PathEscape(s.Name)),
					 Title:   fmt.Sprintf("%s (%d)", s.Name, s.Count),
					 Updated: time.Now().Format(time.RFC3339),
					 Links:   []domain.OPDSLink{{Rel: "subsection", Href: baseURL + "/opds/serie/" + url.PathEscape(s.Name), Type: "application/atom+xml;profile=opds-catalog;kind=acquisition"}},
			})
		}

		opdsNavigationFeed(w, r, "Серии", entries)
	}
}

func opdsSerieHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series := strings.TrimPrefix(r.URL.Path, "/opds/serie/")
		series, _ = url.PathUnescape(series)
		filters := domain.SearchFilters{Series: series}
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

func opdsSearchHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := searchQueryFromRequest(r)
		if query == "" {
			opdsSearchFormFeed(w, r)
			return
		}
		opdsSearchChooserFeed(w, r, dm, query)
	}
}

func opdsSearchPathHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.URL.Path, "/opds/search/q/")
		raw = strings.Trim(raw, "/")
		if raw == "" || raw == "{searchTerms}" {
			opdsSearchFormFeed(w, r)
			return
		}
		query, err := url.PathUnescape(raw)
		if err != nil || strings.TrimSpace(query) == "" {
			http.Error(w, "Некорректный поисковый запрос", http.StatusBadRequest)
			return
		}
		opdsSearchChooserFeed(w, r, dm, query)
	}
}

func opdsSearchResultsHandler(cat *Catalog, dm *storage.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searchType := r.URL.Query().Get("type")
		searchTerm := r.URL.Query().Get("term")
		if searchType == "" || searchTerm == "" {
			http.Error(w, "Отсутствуют параметры type или term", http.StatusBadRequest)
			return
		}

		baseURL := opdsBaseURL(r)
		var entries []domain.OPDSEntry
		var title string

		switch searchType {
			case "author":
				filters := domain.SearchFilters{Author: searchTerm}
				books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
				if err != nil {
					log.Printf("Ошибка поиска по автору: %v", err)
					http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
					return
				}

				seriesMap, standaloneBooks := splitSeriesAndStandalone(books)

				entries = append(entries, buildSeriesSubsectionEntries(seriesMap, seriesEntrySpec{
					IDPrefix:    fmt.Sprintf("search-author-series-%s", url.PathEscape(searchTerm)),
					TitlePrefix: "Серия: ",
					HrefFor: func(seriesName string) string {
						return baseURL + "/opds/serie/" + url.PathEscape(seriesName) + "?author=" + url.PathEscape(searchTerm)
					},
				})...)

				if len(standaloneBooks) > 0 {
					entries = append(entries, buildStandaloneSubsectionEntry(
						fmt.Sprintf("urn:uuid:search-author-standalone-%s", url.PathEscape(searchTerm)),
						fmt.Sprintf("Отдельные книги (%d)", len(standaloneBooks)),
						baseURL+"/opds/author-standalone/"+url.PathEscape(searchTerm),
					))
				}

				title = fmt.Sprintf("Поиск по автору: %s (найдено %d книг)", searchTerm, total)

				case "title":
					filters := domain.SearchFilters{Title: searchTerm}
					books, total, err := dm.AdvancedSearchBooks(filters, 1, 1000)
					if err != nil {
						log.Printf("Ошибка поиска по названию: %v", err)
						http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
						return
					}

					seriesMap, exactTitleBooks := splitSeriesAndStandalone(books)

					entries = append(entries, buildSeriesSubsectionEntries(seriesMap, seriesEntrySpec{
						IDPrefix:    fmt.Sprintf("search-title-series-%s", url.PathEscape(searchTerm)),
						TitlePrefix: "В серии: ",
						HrefFor: func(seriesName string) string {
							return baseURL + "/opds/title-in-series/" + url.PathEscape(searchTerm) + "?series=" + url.PathEscape(seriesName)
						},
					})...)

					if len(exactTitleBooks) > 0 {
						entries = append(entries, buildStandaloneSubsectionEntry(
							fmt.Sprintf("urn:uuid:search-title-exact-%s", url.PathEscape(searchTerm)),
							fmt.Sprintf("Книги с названием '%s' (%d)", searchTerm, len(exactTitleBooks)),
							baseURL+"/opds/title-exact/"+url.PathEscape(searchTerm),
						))
					}

					title = fmt.Sprintf("Поиск по названию: %s (найдено %d книг)", searchTerm, total)

					case "series":
						// Получаем все книги, соответствующие поиску по сериям
						filters := domain.SearchFilters{Series: searchTerm}
						books, _, err := dm.AdvancedSearchBooks(filters, 1, 10000)
						if err != nil {
							log.Printf("Ошибка поиска по сериям: %v", err)
							http.Error(w, "Ошибка поиска", http.StatusInternalServerError)
							return
						}

						// Группируем книги по сериям
						seriesMap, _ := splitSeriesAndStandalone(books)

						// Преобразуем map в slice для сортировки
						type seriesInfo struct {
							Name  string
							Books []domain.Book
						}
						var seriesList []seriesInfo
						for name, books := range seriesMap {
							seriesList = append(seriesList, seriesInfo{Name: name, Books: books})
						}

						// Сортируем серии по релевантности
						sort.Slice(seriesList, func(i, j int) bool {
							pi := storage.GetTitlePriority(seriesList[i].Name, searchTerm)
							pj := storage.GetTitlePriority(seriesList[j].Name, searchTerm)

							if pi != pj {
								return pi < pj
							}
							return strings.ToLower(seriesList[i].Name) < strings.ToLower(seriesList[j].Name)
						})

						// Создаем навигационные записи для каждой найденной серии
						for _, series := range seriesList {
							entries = append(entries, domain.OPDSEntry{
								ID:      fmt.Sprintf("urn:uuid:search-series-%s", url.PathEscape(series.Name)),
									 Title:   fmt.Sprintf("%s (%d книг)", series.Name, len(series.Books)),
									 Updated: time.Now().Format(time.RFC3339),
									 Links: []domain.OPDSLink{{
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

func opdsOpenSearchHandler(cat *Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/opensearchdescription+xml")
		baseURL := opdsBaseURL(r)
		pathTemplate := opdsSearchURLTemplate(baseURL)
		queryTemplate := baseURL + "/opds/search?term={searchTerms}"
		desc := domain.OpenSearchDescription{
			XMLNS:          "http://a9.com/-/spec/opensearch/1.1/",
			ShortName:      "inpx-web",
			Description:    "Поиск по каталогу",
			InputEncoding:  "UTF-8",
			OutputEncoding: "UTF-8",
			URLs: []domain.OpenSearchURL{
				{
					Type:     "application/atom+xml;profile=opds-catalog",
					Template: pathTemplate,
				},
				{
					Type:     "application/atom+xml;profile=opds-catalog",
					Template: queryTemplate,
				},
			},
		}
		writeXMLResponse(w, desc)
	}
}
