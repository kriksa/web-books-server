package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func NewSystemManager() *SystemManager {
	return &SystemManager{
		ParseStatus: ParseStatus{Message: "Инициализация..."},
	}
}

func (sm *SystemManager) ReloadServices(triggerParse bool) error {
	log.Println("SystemManager: Перезагрузка сервисов...")

	sm.StatusMu.Lock()
	sm.ParseStatus.Message = "Загрузка конфигурации..."
	sm.StatusMu.Unlock()

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("Ошибка загрузки конфига: %v", err)
		return err
	}

	sm.Mu.Lock()
	sm.Config = cfg

	if sm.DB != nil {
		sm.DB.Close()
		sm.DB = nil
	}
	sm.Mu.Unlock()

	sm.StatusMu.Lock()
	sm.ParseStatus.Message = "Подключение к БД..."
	sm.StatusMu.Unlock()

	var newDM *DBManager
	newDM, err = NewDBManager(cfg)
	if err != nil {
		log.Printf("Ошибка подключения к БД: %v", err)
	}

	sm.Mu.Lock()
	sm.DB = newDM
	sm.Mu.Unlock()

	if triggerParse && newDM != nil {
		go sm.RunParserBackground()
	} else if newDM != nil {
		inpxFile, err := cfg.FindLatestINPX()
		if err == nil {
			info, _ := os.Stat(inpxFile)
			mtime := float64(info.ModTime().Unix())
			dbMtime, _ := newDM.GetInpxMtime()
			bookCount, _ := newDM.GetBookCount()

			// Запускаем парсинг только если:
			// 1. БД пустая (bookCount == 0) ИЛИ
			// 2. INPX обновился (mtime > dbMtime)
			if bookCount == 0 || mtime > dbMtime {
				if bookCount == 0 {
					log.Println("БД пустая, запуск парсинга...")
				} else {
					log.Println("Обнаружен новый INPX, запуск фонового парсинга...")
				}
				go sm.RunParserBackground()
			} else {
				log.Println("БД заполнена и INPX не обновился, парсинг не требуется")
			}
		}
	}

	sm.StatusMu.Lock()
	if !sm.ParseStatus.IsParsing {
		sm.ParseStatus.Message = "Готов к работе"
		if sm.DB == nil {
			sm.ParseStatus.Message = "Укажите папку с книгами в настройках"
		}
	}
	sm.StatusMu.Unlock()

	return nil
}

func (sm *SystemManager) RunParserBackground() {
	sm.StatusMu.Lock()
	if sm.ParseStatus.IsParsing {
		sm.StatusMu.Unlock()
		return
	}
	sm.ParseStatus.IsParsing = true
	sm.ParseStatus.Progress = 0
	sm.ParseStatus.Total = 0
	sm.ParseStatus.Message = "Обновление базы книг"
	sm.ParseStatus.Stage = ""
	sm.ParseStatus.CurrentFile = ""
	sm.ParseStatus.StartTime = time.Now().Unix()
	sm.ParseStatus.EstimatedRemainingSec = 0
	sm.StatusMu.Unlock()

	defer func() {
		sm.StatusMu.Lock()
		sm.ParseStatus.IsParsing = false
		sm.ParseStatus.Message = "Парсинг завершен"
		sm.StatusMu.Unlock()
	}()

	sm.Mu.RLock()
	cfg := sm.Config
	dm := sm.DB
	sm.Mu.RUnlock()

	if dm == nil {
		return
	}

	inpxFile, err := cfg.FindLatestINPX()
	if err != nil {
		log.Printf("Ошибка поиска INPX: %v", err)
		sm.StatusMu.Lock()
		sm.ParseStatus.Message = "Ошибка: INPX не найден"
		sm.StatusMu.Unlock()
		return
	}

	info, err := os.Stat(inpxFile)
	if err != nil {
		return
	}
	mtime := float64(info.ModTime().Unix())

	// Проверяем, нужно ли выполнять парсинг
	dbMtime, _ := dm.GetInpxMtime()
	bookCount, _ := dm.GetBookCount()

	// Если БД заполнена и INPX не обновился, не запускаем парсинг
	if bookCount > 0 && mtime <= dbMtime {
		log.Println("Парсинг не требуется: БД заполнена и INPX не обновился")
		sm.StatusMu.Lock()
		sm.ParseStatus.Message = "Парсинг не требуется"
		sm.StatusMu.Unlock()
		return
	}

	parser := NewParser(cfg, dm)

	parser.onTotal = func(total int64) {
		sm.StatusMu.Lock()
		sm.ParseStatus.Total = total
		sm.StatusMu.Unlock()
	}

	parser.onStage = func(stage, currentFile string) {
		sm.StatusMu.Lock()
		sm.ParseStatus.Stage = stage
		sm.ParseStatus.CurrentFile = currentFile
		switch stage {
		case "count":
			sm.ParseStatus.Message = "Подсчет общего прогресса"
			sm.ParseStatus.EstimatedRemainingSec = 0
		case "parse":
			sm.ParseStatus.Message = "Парсинг INPX"
			// ETA считаем только для фазы основного чтения/парсинга INP.
			// Прогресс измеряется в байтах распакованного текста.
			sm.ParseStatus.StartTime = time.Now().Unix()
		case "parse_file":
			sm.ParseStatus.Message = "Парсинг INPX"
		default:
			sm.ParseStatus.Message = "Обновление базы книг"
		}
		sm.StatusMu.Unlock()
	}

	// Минимальный прогресс для показа ETA (чтобы оценка не прыгала в самом начале),
	// теперь прогресс измеряется в байтах.
	const minProgressForETA = int64(1 * 1024 * 1024) // 1 МБ
	// Доля времени на сохранение в БД (~10%) — приблизительная оценка до конца всего парсинга
	const savePhaseFraction = 0.1

	// onProgress получает суммарно обработанные байты (по всем INP-файлам).
	parser.onProgress = func(processedBytes int64) {
		sm.StatusMu.Lock()
		sm.ParseStatus.Progress = processedBytes
		sm.StatusMu.Unlock()

		// Обновляем ETA отдельно, чтобы не блокировать мьютекс
		sm.StatusMu.Lock()
		total := sm.ParseStatus.Total
		if total > 0 && processedBytes >= minProgressForETA {
			elapsed := time.Now().Unix() - sm.ParseStatus.StartTime
			if elapsed > 0 {
				remaining := total - processedBytes
				if remaining > 0 {
					// ETA в секундах: remainingBytes / (processedBytes / elapsedSeconds)
					etaParse := int(remaining * elapsed / processedBytes)
					// Добавляем ~10% на этап сохранения в БД
					sm.ParseStatus.EstimatedRemainingSec = int(float64(etaParse) * (1 + savePhaseFraction))
				} else {
					sm.ParseStatus.EstimatedRemainingSec = 0
				}
			}
		} else {
			sm.ParseStatus.EstimatedRemainingSec = 0
		}
		sm.StatusMu.Unlock()
	}

	log.Println("Запуск парсинга INPX...")
	parser.StartWorkers()
	if err := parser.ParseINPX(inpxFile); err != nil {
		log.Printf("Ошибка парсинга: %v", err)
		sm.StatusMu.Lock()
		sm.ParseStatus.Message = "Ошибка парсинга"
		sm.StatusMu.Unlock()
		return
	}

	if err := dm.UpdateInpxMtime(mtime); err != nil {
		log.Printf("Ошибка сохранения времени INPX: %v", err)
	}

	log.Println("Парсинг успешно завершен")
}

func (c *customFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contentTypes := map[string]string{
		".js":    "application/javascript",
		".css":   "text/css",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".svg":   "image/svg+xml",
		".ico":   "image/x-icon",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".eot":   "application/vnd.ms-fontobject",
	}

	for ext, contentType := range contentTypes {
		if strings.HasSuffix(r.URL.Path, ext) {
			w.Header().Set("Content-Type", contentType)
			break
		}
	}

	w.Header().Del("X-Content-Type-Options")
	http.FileServer(c.root).ServeHTTP(w, r)
}

func startWebServer(sm *SystemManager) {
	sm.ReloadServices(false)

	sm.Mu.RLock()
	port := sm.Config.Port
	opdsRoot := sm.Config.OPDSRoot
	signingKey := sm.Config.JWTSigningKey
	sm.Mu.RUnlock()

	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	mux := http.NewServeMux()

	// API статуса приложения
	mux.HandleFunc("/api/app-status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sm.StatusMu.RLock()
		status := sm.ParseStatus
		sm.StatusMu.RUnlock()

		sm.Mu.RLock()
		dbReady := sm.DB != nil
		sm.Mu.RUnlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":                  "ready",
			"db_ready":                dbReady,
			"is_parsing":              status.IsParsing,
			"progress":                status.Progress,
			"total":                   status.Total,
			"message":                 status.Message,
			"stage":                   status.Stage,
			"current_file":            status.CurrentFile,
			"estimated_remaining_sec": status.EstimatedRemainingSec,
		})
	})

	// Перезапуск приложения
	mux.HandleFunc("/api/restart", func(w http.ResponseWriter, r *http.Request) {
		go sm.ReloadServices(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})
	})

	if signingKey == nil {
		log.Fatal("Критическая ошибка: Ключ JWT (JWTSigningKey) отсутствует.")
	}

	// Middleware для аутентификации
	authMiddleware := jwtAuthMiddleware(signingKey)

	// Аутентификация и настройка
	mux.HandleFunc("/api/setup-status", handleSetupStatus(sm))
	mux.HandleFunc("/api/setup", handleSetup(sm, signingKey))
	mux.HandleFunc("/api/login", handleLogin(sm, signingKey))

	// Защищенные эндпоинты (требуется аутентификация)
	mux.Handle("/api/user/status", authMiddleware(http.HandlerFunc(handleUserStatus)))
	mux.Handle("/api/change-password", authMiddleware(http.HandlerFunc(handleChangePassword(sm))))
	mux.Handle("/api/favorites", authMiddleware(http.HandlerFunc(handleFavorites(sm))))
	mux.Handle("/api/favorites/books", authMiddleware(http.HandlerFunc(handleFavoritesBooks(sm))))

	// Администраторские эндпоинты (требуется роль admin)
	mux.Handle("/api/config", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(configHandler(sm)))))
	mux.Handle("/api/reset-password", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleResetPassword(sm)))))
	mux.Handle("/api/update-profile", authMiddleware(http.HandlerFunc(handleUpdateProfile(sm))))
	mux.Handle("/api/users", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleListUsers(sm)))))
	mux.Handle("/api/users/create", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleCreateUser(sm)))))
	mux.Handle("/api/users/delete", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleDeleteUser(sm)))))
	mux.Handle("/api/library-stats", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleLibraryStats(sm)))))

	// Настройки читалки (публичный доступ)
	mux.HandleFunc("/api/reader-config", readerConfigHandler)

	// Веб-аутентификация
	mux.HandleFunc("/api/web-auth-status", webAuthStatusHandler)
	mux.HandleFunc("/api/web-auth", webAuthHandler)

	// Обертка для обработчиков, требующих доступ к БД
	wrapDB := func(h func(*DBManager) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			dm := sm.GetDB()
			if dm == nil {
				http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
				return
			}
			h(dm)(w, r)
		}
	}

	// OPDS каталог
	mux.HandleFunc("/opds/", opdsRootHandler)
	mux.HandleFunc("/opds/opensearch", opdsOpenSearchHandler())
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
	mux.HandleFunc("/opds/search", wrapDB(opdsSearchHandler))

	// Поиск книг
	mux.HandleFunc("/api/search", wrapDB(apiSearchHandler))

	// Детали книги
	mux.HandleFunc("/api/book/details", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm != nil {
			apiBookDetailsHandler(dm, dir)(w, r)
		} else {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
		}
	})

	// Обложки книг
	mux.HandleFunc("/api/cover", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm != nil {
			apiCoverHandler(dm, dir)(w, r)
		} else {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
		}
	})

	// Скачивание книг
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm != nil {
			downloadHandler(dm, dir)(w, r)
		} else {
			http.Error(w, "DB Not Ready", http.StatusServiceUnavailable)
		}
	})

	// Статические файлы фронтенда
	distFS, err := fs.Sub(frontendFiles, "frontend/dist")
	if err != nil {
		log.Fatalf("Err dist fs: %v", err)
	}
	assetsFS, err := fs.Sub(distFS, "assets")
	if err != nil {
		log.Fatalf("Err assets fs: %v", err)
	}

	httpAssetsFS := http.FS(assetsFS)
	mux.Handle("/assets/", http.StripPrefix("/assets/", &customFileServer{root: httpAssetsFS}))

	// Главная страница SPA
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/assets/") ||
			strings.HasPrefix(r.URL.Path, "/download") || strings.HasPrefix(r.URL.Path, "/opds/") {
				http.NotFound(w, r)
				return
			}
			content, err := fs.ReadFile(distFS, "index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(content)
	})

	server.Handler = mux
	log.Printf("Веб-сервер запущен на порту :%s", port)
	log.Printf("OPDS доступен: http://localhost:%s%s", port, opdsRoot)
	log.Fatal(server.ListenAndServe())
}

func (sm *SystemManager) GetDB() *DBManager {
	sm.Mu.RLock()
	defer sm.Mu.RUnlock()
	return sm.DB
}
