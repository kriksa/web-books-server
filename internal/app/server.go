package app
import (
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"web_books/internal/domain"
	"web_books/internal/httpapi"
	"web_books/internal/opds"
	"web_books/internal/spaembed"
)

// Run starts the web books HTTP server (blocks until exit).
func Run() {
	startWebServer(NewSystemManager())
}

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
		inpxFile, err := FindLatestINPX(cfg)
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

	inpxFile, err := FindLatestINPX(cfg)
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
		".mjs":   "application/javascript",
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

	ext := strings.ToLower(filepath.Ext(r.URL.Path))
	if contentType, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", contentType)
	} else if guessed := mime.TypeByExtension(ext); guessed != "" {
		w.Header().Set("Content-Type", guessed)
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

	// Браузеры по умолчанию запрашивают /favicon.ico — отдаём редирект на SVG из сборки.
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/favicon.svg", http.StatusFound)
	})

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
	mux.HandleFunc("/api/reading-progress", handleReadingProgressAPI(sm))

	// Администраторские эндпоинты (требуется роль admin)
	mux.Handle("/api/reset-password", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleResetPassword(sm)))))
	mux.Handle("/api/update-profile", authMiddleware(http.HandlerFunc(handleUpdateProfile(sm))))
	mux.Handle("/api/users", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleListUsers(sm)))))
	mux.Handle("/api/users/create", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleCreateUser(sm)))))
	mux.Handle("/api/users/delete", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleDeleteUser(sm)))))
	mux.Handle("/api/library-stats", authMiddleware(adminOnlyMiddleware(http.HandlerFunc(handleLibraryStats(sm)))))

	// Liberama compatibility endpoints (public):
	// - WebSocket RPC at /ws (and /liberama/ws alias because Liberama client derives it from pathname)
	// - Upload endpoint and static upload dir
	mux.HandleFunc("/ws", handleLiberamaWS(sm))
	mux.HandleFunc("/liberama/ws", handleLiberamaWS(sm))
	mux.HandleFunc("/api/reader/upload-file", handleLiberamaUploadFile(sm))
	if uploadFS, err := handleUploadStatic(); err == nil {
		mux.Handle("/upload/", http.StripPrefix("/upload/", uploadFS))
	}

	opdsCat := &opds.Catalog{
		ConfigFn: func() *domain.Config {
			sm.Mu.RLock()
			defer sm.Mu.RUnlock()
			return sm.Config
		},
	}
	opds.Register(mux, opdsCat, sm.GetDB)
	httpapi.Register(mux, sm, sm.GetDB, authMiddleware)

	mux.HandleFunc("/api/book/reader-meta", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleReaderMeta(dm, dir)(w, r)
	})
	mux.HandleFunc("/api/reader/foliate-converted.fb2", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleReaderFoliateConvertedFB2(dm, dir)(w, r)
	})
	mux.HandleFunc("/api/book/text", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleBookPlainText(dm, dir)(w, r)
	})
	mux.HandleFunc("/api/book/fb2-model", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleBookFB2Model(dm, dir)(w, r)
	})
	mux.HandleFunc("/api/book/render-model", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleBookRenderModel(dm, dir)(w, r)
	})
	mux.HandleFunc("/api/book/repaginate", handleRepaginateModel())
	mux.HandleFunc("/api/book/docx", func(w http.ResponseWriter, r *http.Request) {
		sm.Mu.RLock()
		dm := sm.DB
		dir := sm.Config.BooksDir
		sm.Mu.RUnlock()
		if dm == nil {
			http.Error(w, "База данных не подключена или обновляется", http.StatusServiceUnavailable)
			return
		}
		handleBookDocxForReader(dm, dir)(w, r)
	})

	// Статические файлы фронтенда (Vite: index.html, reader.html, assets/)
	distFS, err := fs.Sub(spaembed.FS, "spa")
	if err != nil {
		log.Fatalf("Err dist fs: %v", err)
	}

	// SPA routes must return index.html (otherwise /reader 404s because it's not a real file).
	serveIndex := func(w http.ResponseWriter) {
		content, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(content)
	}
	mux.HandleFunc("/reader", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	mux.HandleFunc("/reader/", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	// Встроенная foliate-читалка: /read?book=… (как отдельный «роут» SPA, без хэша).
	mux.HandleFunc("/read", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	mux.HandleFunc("/read/", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })

	mux.Handle("/", &customFileServer{root: http.FS(distFS)})

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
