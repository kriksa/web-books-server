package httpapi

import (
	"encoding/json"
	"net/http"

	"web_books/internal/auth"
)

// Register mounts public catalog and admin JSON API routes.
func Register(mux *http.ServeMux, host Host, db DBAccessor, jwtAuth func(http.Handler) http.Handler) {
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		dm := db()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		apiSearchHandler(host, dm)(w, r)
	})

	mux.HandleFunc("/api/book/details", func(w http.ResponseWriter, r *http.Request) {
		dm := db()
		if dm == nil {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
			return
		}
		apiBookDetailsHandler(dm, host.BooksDir())(w, r)
	})

	mux.HandleFunc("/api/cover", func(w http.ResponseWriter, r *http.Request) {
		dm := db()
		if dm == nil {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
			return
		}
		apiCoverHandler(dm, host.BooksDir())(w, r)
	})

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		dm := db()
		if dm == nil {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
			return
		}
		downloadHandler(dm, host.BooksDir())(w, r)
	})

	mux.HandleFunc("/api/reader-config", readerConfigHandler)
	mux.HandleFunc("/api/web-auth-status", webAuthStatusHandler)
	mux.HandleFunc("/api/web-auth", webAuthHandler)

	if jwtAuth != nil {
		mux.Handle("/api/config", jwtAuth(auth.AdminOnly(http.HandlerFunc(configHandler(host)))))
		mux.Handle("/api/restart", jwtAuth(auth.AdminOnly(http.HandlerFunc(restartHandler(host)))))
	}
}

func restartHandler(host Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не разрешён", http.StatusMethodNotAllowed)
			return
		}
		go host.ReloadServices(true)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})
	}
}
