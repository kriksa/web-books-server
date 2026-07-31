package opds

import (
	"net/http"

	"web_books/internal/storage"
)

// DBAccessor returns the library database or nil while reloading.
type DBAccessor func() *storage.Manager

// Register mounts OPDS routes on mux.
func Register(mux *http.ServeMux, cat *Catalog, db DBAccessor) {
	wrapDB := func(h func(*Catalog, *storage.Manager) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			dm := db()
			if dm == nil {
				http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
				return
			}
			h(cat, dm)(w, r)
		}
	}

	mux.HandleFunc("/opds", opdsRootHandler(cat))
	mux.HandleFunc("/opds/", opdsRootHandler(cat))
	mux.HandleFunc("/opds/opensearch", opdsOpenSearchHandler(cat))
	mux.HandleFunc("/opds/new", wrapDB(opdsNewHandler))
	mux.HandleFunc("/opds/authors", wrapDB(opdsAuthorsHandler))
	mux.HandleFunc("/opds/author/", wrapDB(opdsAuthorHandler))
	mux.HandleFunc("/opds/author-standalone/", wrapDB(opdsAuthorStandaloneHandler))
	mux.HandleFunc("/opds/series", wrapDB(opdsSeriesHandler))
	mux.HandleFunc("/opds/serie/", wrapDB(opdsSerieHandler))
	mux.HandleFunc("/opds/titles", wrapDB(opdsTitlesHandler))
	mux.HandleFunc("/opds/title/", wrapDB(opdsTitleHandler))
	mux.HandleFunc("/opds/title-exact/", wrapDB(opdsTitleExactHandler))
	mux.HandleFunc("/opds/title-in-series/", wrapDB(opdsTitleInSeriesHandler))
	mux.HandleFunc("/opds/search-results", wrapDB(opdsSearchResultsHandler))
	mux.HandleFunc("/opds/search/q/", wrapDB(opdsSearchPathHandler))
	mux.HandleFunc("/opds/search", wrapDB(opdsSearchHandler))
}
