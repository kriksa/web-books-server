package storage

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"web_books/internal/bookutil"
	"web_books/internal/domain"

	_ "modernc.org/sqlite"
)

// db.go — слой доступа к SQLite.
//
// Основные принципы:
// - Используем WAL (write-ahead log) для более стабильной работы при конкурентных запросах.
// - Включаем foreign_keys и настраиваем busy_timeout, чтобы уменьшить ошибки "database is locked".
// - Создаём все таблицы/индексы при старте (idempotent через IF NOT EXISTS).

const dbFileName = "library.db"

// Manager is the SQLite library database handle.
type Manager struct {
	db *sql.DB
	mu sync.RWMutex
}

func New(cfg *domain.Config) (*Manager, error) {
	// Количество соединений SQLite ограничиваем в зависимости от CPU,
	// чтобы не создавать лишнюю конкуренцию за блокировки.
	maxWorkers := runtime.NumCPU()
	var dbPath string
	if testPath := os.Getenv("TEST_DB_PATH"); testPath != "" {
		dbPath = testPath
		os.MkdirAll(filepath.Dir(testPath), 0755)
	} else {
		configDir := "config"
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return nil, fmt.Errorf("не удалось создать папку config: %v", err)
		}
		dbPath = filepath.Join(configDir, dbFileName)
	}
	escapedPath := url.PathEscape(dbPath)
	// busy_timeout: пересборка FTS на большой библиотеке держит write-транзакцию долго.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(60000)&_journal_mode=WAL", escapedPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к БД: %v", err)
	}

	db.SetMaxOpenConns(maxWorkers * 2)
	db.SetMaxIdleConns(maxWorkers)
	db.SetConnMaxLifetime(0)

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	dm := &Manager{db: db}
	return dm, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS authors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS genres (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS series (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			series_id INTEGER,
			series_no INTEGER DEFAULT 0,
			file_name TEXT NOT NULL,
			zip TEXT NOT NULL,
			format TEXT NOT NULL,
			file_size INTEGER,
			language TEXT NOT NULL DEFAULT 'unknown',
			del INTEGER DEFAULT 0,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (series_id) REFERENCES series(id) ON DELETE SET NULL,
			UNIQUE(title, file_name, zip)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_books_lang ON books(language);`,
		`CREATE INDEX IF NOT EXISTS idx_books_del_lang ON books(del, language);`,
		`CREATE INDEX IF NOT EXISTS idx_books_added ON books(added_at);`,
		`CREATE INDEX IF NOT EXISTS idx_books_series ON books(series_id);`,
		`CREATE TABLE IF NOT EXISTS book_authors (
			book_id INTEGER NOT NULL,
			author_id INTEGER NOT NULL,
			PRIMARY KEY (book_id, author_id),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
			FOREIGN KEY (author_id) REFERENCES authors(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_book_authors_author ON book_authors(author_id);`,
		`CREATE TABLE IF NOT EXISTS book_genres (
			book_id INTEGER NOT NULL,
			genre_id INTEGER NOT NULL,
			PRIMARY KEY (book_id, genre_id),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
			FOREIGN KEY (genre_id) REFERENCES genres(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_book_genres_genre_id ON book_genres(genre_id);`,
		`CREATE INDEX IF NOT EXISTS idx_book_genres_book_id ON book_genres(book_id);`,
		`CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS user_favorites (
			user_id INTEGER NOT NULL,
			book_id INTEGER NOT NULL,
			PRIMARY KEY (user_id, book_id),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_favorites_user ON user_favorites(user_id);`,
		`CREATE TABLE IF NOT EXISTS reading_progress (
			user_id INTEGER NOT NULL,
			book_id INTEGER NOT NULL,
			position TEXT NOT NULL DEFAULT '{}',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, book_id),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_reading_progress_book ON reading_progress(book_id);`,
		`CREATE TABLE IF NOT EXISTS reading_progress_guest (
			guest_key TEXT NOT NULL,
			book_id INTEGER NOT NULL,
			position TEXT NOT NULL DEFAULT '{}',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (guest_key, book_id),
			FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_reading_progress_guest_book ON reading_progress_guest(book_id);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
			title,
			author,
			series,
			content=''
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("ошибка создания таблицы: %v (query: %s)", err, q)
		}
	}
	if err := migrateFTSIndexing(db); err != nil {
		// Не валим подключение к БД: иначе весь API (503 «БД не подключена») падает из‑за FTS.
		log.Printf("FTS: миграция/пересборка books_fts не выполнена (поиск по названию/серии через FTS может быть неполным): %v", err)
	}
	return nil
}

// ftsSchemaVersionBooksFTS — PRAGMA user_version: пересборка books_fts при смене правил индексации.
// v3: searchNormFold = Unicode lower + trim (без ё→е), как у tokenizeForFTS для MATCH.
const ftsSchemaVersionBooksFTS = 3

func rebuildBooksFTS(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM books_fts`); err != nil {
		return fmt.Errorf("books_fts DELETE: %w", err)
	}
	rows, err := tx.Query(`
		SELECT b.id, b.title,
			(SELECT GROUP_CONCAT(a.name, ', ') FROM book_authors ba JOIN authors a ON ba.author_id = a.id WHERE ba.book_id = b.id) AS authors,
			COALESCE(s.name, '') AS series
		FROM books b
		LEFT JOIN series s ON b.series_id = s.id
	`)
	if err != nil {
		return fmt.Errorf("rebuildBooksFTS SELECT: %w", err)
	}
	defer rows.Close()

	stmt, err := tx.Prepare(`INSERT INTO books_fts (rowid, title, author, series) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var id int64
		var title string
		var authors, series sql.NullString
		if err := rows.Scan(&id, &title, &authors, &series); err != nil {
			return fmt.Errorf("rebuildBooksFTS scan: %w", err)
		}
		auth := ""
		if authors.Valid {
			auth = authors.String
		}
		sr := ""
		if series.Valid {
			sr = series.String
		}
		if _, err := stmt.Exec(id, ftsIndexText(title), ftsIndexText(auth), ftsIndexText(sr)); err != nil {
			return fmt.Errorf("books_fts INSERT rowid=%d: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func migrateFTSIndexing(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return fmt.Errorf("PRAGMA user_version: %w", err)
	}
	if v >= ftsSchemaVersionBooksFTS {
		return nil
	}
	log.Printf("FTS: пересборка books_fts (Unicode lower, согласовано с tokenizer MATCH)...")
	if err := rebuildBooksFTS(db); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, ftsSchemaVersionBooksFTS)); err != nil {
		return fmt.Errorf("PRAGMA user_version =: %w", err)
	}
	return nil
}

// ==================== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ====================

func tokenizeForFTS(s string) []string {
	base := searchNormFold(s)
	if base == "" {
		return nil
	}

	var tokens []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		tokens = append(tokens, buf.String())
		buf.Reset()
	}

	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func buildFtsQuery(column, text string) string {
	tokens := tokenizeForFTS(text)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		parts = append(parts, fmt.Sprintf("%s:%s*", column, t))
	}
	return strings.Join(parts, " AND ")
}

func normalizeAuthorName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if strings.Contains(name, ",") {
		parts := strings.SplitN(name, ",", 2)
		if len(parts) == 2 {
			lastName := strings.TrimSpace(parts[0])
			firstName := strings.TrimSpace(parts[1])
			return firstName + " " + lastName
		}
	}
	return name
}

// SQLite LOWER() is ASCII-only в типичных сборках: для кириллицы приводим регистр в Go.
var unicodeLowerUnd = cases.Lower(language.Und)

func searchNormFold(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return unicodeLowerUnd.String(s)
}

// ftsIndexText — текст в FTS5: те же правила, что и tokenizeForFTS (searchNormFold + tokenizer).
// Иначе индекс с «Война…», а запрос с «война…» даёт 0 строк MATCH по кириллице.
func ftsIndexText(s string) string {
	return searchNormFold(s)
}

// sqlLikeContains строит шаблон LIKE '%...%' с экранированием метасимволов SQLite.
func sqlLikeContains(q string) string {
	q = strings.ReplaceAll(q, "\\", "\\\\")
	q = strings.ReplaceAll(q, "%", "\\%")
	q = strings.ReplaceAll(q, "_", "\\_")
	return "%" + q + "%"
}

func generateAuthorSearchVariants(name string) []string {
	name = strings.Join(strings.Fields(name), " ")
	variants := []string{name}
	if strings.Contains(name, " ") {
		parts := strings.SplitN(name, " ", 2)
		if len(parts) == 2 {
			variants = append(variants, parts[1]+" "+parts[0])
		}
	}
	return variants
}

// authorQueryNeedles — подстроки для поиска автора (Unicode lower), без кэша и без ё→е.
func authorQueryNeedles(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var needles []string
	add := func(raw string) {
		k := searchNormFold(raw)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		needles = append(needles, k)
	}
	add(q)
	add(normalizeAuthorName(q))
	for _, v := range generateAuthorSearchVariants(q) {
		add(v)
	}
	return needles
}

// relevanceNormMatch: 1 полное совпадение norm, 2 префикс, 3 вхождение, 4 нет.
func relevanceNormMatch(normName string, needles []string) int {
	rel := 4
	for _, nd := range needles {
		if nd == "" {
			continue
		}
		if normName == nd {
			return 1
		}
		if strings.HasPrefix(normName, nd) {
			if rel > 2 {
				rel = 2
			}
		} else if strings.Contains(normName, nd) {
			if rel > 3 {
				rel = 3
			}
		}
	}
	return rel
}

// ==================== СОХРАНЕНИЕ ДАННЫХ ====================

func (dm *Manager) SaveBooksBatch(books []domain.Book) error {
	if len(books) == 0 {
		return nil
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()

	tx, err := dm.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Собираем уникальные сущности
	uniqueAuthors := make(map[string]int64)
	uniqueGenres := make(map[string]int64)
	uniqueSeries := make(map[string]int64)

	for _, b := range books {
		if b.Series != "" {
			uniqueSeries[b.Series] = 0
		}
		for _, a := range b.AuthorsList {
			uniqueAuthors[a] = 0
		}
		for _, g := range b.GenresList {
			uniqueGenres[g] = 0
		}
	}

	// Получаем ID для всех сущностей
	if len(uniqueAuthors) > 0 {
		if err := dm.resolveIds(tx, "authors", "name", uniqueAuthors); err != nil {
			log.Printf("Ошибка вставки авторов: %v", err)
		}
	}
	if len(uniqueGenres) > 0 {
		if err := dm.resolveIds(tx, "genres", "code", uniqueGenres); err != nil {
			log.Printf("Ошибка вставки жанров: %v", err)
		}
	}
	if len(uniqueSeries) > 0 {
		if err := dm.resolveIds(tx, "series", "name", uniqueSeries); err != nil {
			log.Printf("Ошибка вставки серий: %v", err)
		}
	}

	// Подготавливаем statements
	bookStmt, err := tx.Prepare(`INSERT OR IGNORE INTO books
		(title, series_id, series_no, file_name, zip, format, file_size, language, del, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer bookStmt.Close()

	// Отдельный стейтмент для поиска существующей книги
	findBookStmt, err := tx.Prepare(`SELECT id FROM books 
		WHERE title = ? AND file_name = ? AND zip = ?`)
	if err != nil {
		return err
	}
	defer findBookStmt.Close()

	authorLinkStmt, err := tx.Prepare(`INSERT OR IGNORE INTO book_authors (book_id, author_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer authorLinkStmt.Close()

	genreLinkStmt, err := tx.Prepare(`INSERT OR IGNORE INTO book_genres (book_id, genre_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer genreLinkStmt.Close()

	ftsStmt, err := tx.Prepare(`INSERT OR REPLACE INTO books_fts (rowid, title, author, series)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ftsStmt.Close()

	// Сохраняем книги
	for _, b := range books {
		var seriesID interface{}
		if b.Series != "" {
			if id := uniqueSeries[b.Series]; id > 0 {
				seriesID = id
			}
		}

		// Пытаемся вставить книгу
		res, err := bookStmt.Exec(b.Title, seriesID, b.SeriesNo, b.FileName, b.Zip, b.Format, b.FileSize, b.Language, b.Del, b.AddedAt)
		if err != nil {
			log.Printf("Error inserting domain.Book %s: %v", b.Title, err)
			continue
		}

		var bookID int64
		
		// Проверяем, сколько строк реально затронуто
		affected, err := res.RowsAffected()
		if err != nil {
			log.Printf("Error getting rows affected for %s: %v", b.Title, err)
			continue
		}

		if affected > 0 {
			// Книга была вставлена - получаем новый ID
			bookID, err = res.LastInsertId()
			if err != nil {
				log.Printf("Error getting last insert id for %s: %v", b.Title, err)
				continue
			}
			log.Printf("[DEBUG] Inserted new domain.Book: %s, new ID=%d", b.Title, bookID)
		} else {
			// Книга уже существовала - находим её старый ID
			err = findBookStmt.QueryRow(b.Title, b.FileName, b.Zip).Scan(&bookID)
			if err != nil {
				log.Printf("Error finding existing domain.Book %s: %v", b.Title, err)
				continue
			}
			log.Printf("[DEBUG] Found existing domain.Book: %s, existing ID=%d", b.Title, bookID)
		}

		if bookID == 0 {
			log.Printf("WARNING: bookID is 0 for %s", b.Title)
			continue
		}

		// Связываем с авторами
		for _, a := range b.AuthorsList {
			if aID, ok := uniqueAuthors[a]; ok && aID > 0 {
				if _, err := authorLinkStmt.Exec(bookID, aID); err != nil {
					if !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
						log.Printf("Ошибка связи автора %s для книги %s: %v", a, b.Title, err)
					}
				}
			}
		}

		// Связываем с жанрами
		for _, g := range b.GenresList {
			if gID, ok := uniqueGenres[g]; ok && gID > 0 {
				if _, err := genreLinkStmt.Exec(bookID, gID); err != nil {
					if !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
						log.Printf("Ошибка связи жанра %s для книги %s: %v", g, b.Title, err)
					}
				}
			}
		}

		// Обновляем FTS
		authorsText := strings.Join(b.AuthorsList, ", ")
		if _, err := ftsStmt.Exec(bookID, ftsIndexText(b.Title), ftsIndexText(authorsText), ftsIndexText(b.Series)); err != nil {
			log.Printf("Ошибка обновления FTS для %s: %v", b.Title, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (dm *Manager) resolveIds(tx *sql.Tx, table, col string, m map[string]int64) error {
	if len(m) == 0 {
		return nil
	}
	
	// Вставляем новые записи
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}

	chunkSize := 2000
	for i := 0; i < len(names); i += chunkSize {
		end := i + chunkSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[i:end]

		vals := make([]interface{}, 0, len(batch))
		placeholders := make([]string, 0, len(batch))
		for _, val := range batch {
			placeholders = append(placeholders, "(?)")
			vals = append(vals, val)
		}
		query := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES %s", table, col, strings.Join(placeholders, ","))
		if _, err := tx.Exec(query, vals...); err != nil {
			return fmt.Errorf("resolveIds insert error: %v", err)
		}
	}

	// Получаем ID для всех записей
	query := fmt.Sprintf("SELECT id, %s FROM %s WHERE %s IN (", col, table, col)

	for i := 0; i < len(names); i += chunkSize {
		end := i + chunkSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[i:end]

		args := make([]interface{}, len(batch))
		qParts := make([]string, len(batch))
		for j, v := range batch {
			qParts[j] = "?"
			args[j] = v
		}

		q := query + strings.Join(qParts, ",") + ")"
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}

		for rows.Next() {
			var id int64
			var val string
			if err := rows.Scan(&id, &val); err != nil {
				rows.Close()
				return err
			}
			m[val] = id
		}
		rows.Close()
	}
	return nil
}

// ==================== ПОИСК ID СУЩНОСТЕЙ ====================

func (dm *Manager) findEntityIDsByNormSearch(table, errLabel string, needles []string) ([]int64, error) {
	if len(needles) == 0 {
		return []int64{}, nil
	}
	rows, err := dm.db.Query(fmt.Sprintf(`SELECT id, name FROM %s ORDER BY name`, table))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", errLabel, err)
	}
	defer rows.Close()

	type scored struct {
		id        int64
		name      string
		relevance int
	}
	var scoredRows []scored
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		rel := relevanceNormMatch(searchNormFold(name), needles)
		if rel >= 4 {
			continue
		}
		scoredRows = append(scoredRows, scored{id: id, name: name, relevance: rel})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		if scoredRows[i].relevance != scoredRows[j].relevance {
			return scoredRows[i].relevance < scoredRows[j].relevance
		}
		return scoredRows[i].name < scoredRows[j].name
	})
	if len(scoredRows) > 800 {
		scoredRows = scoredRows[:800]
	}
	ids := make([]int64, len(scoredRows))
	for i := range scoredRows {
		ids[i] = scoredRows[i].id
	}
	return ids, nil
}

func (dm *Manager) findAuthorIDsBySearch(query string) ([]int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []int64{}, nil
	}
	needles := authorQueryNeedles(query)
	if len(needles) == 0 {
		return []int64{}, nil
	}
	return dm.findEntityIDsByNormSearch("authors", "ошибка поиска авторов", needles)
}

// findSeriesIDsBySearch — подстрока в имени серии (Unicode lower в Go; SQLite LIKE с кириллицей ненадёжен).
func (dm *Manager) findSeriesIDsBySearch(query string) ([]int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []int64{}, nil
	}
	needle := searchNormFold(query)
	if needle == "" {
		return []int64{}, nil
	}
	return dm.findEntityIDsByNormSearch("series", "ошибка поиска серий", []string{needle})
}

func (dm *Manager) findGenreIDsBySearch(query string) ([]int64, error) {
	// Для жанров ищем по точному совпадению кода
	likeQuery := "%" + query + "%"
	startWithQuery := query + "%"
	
	rows, err := dm.db.Query(`
		SELECT id, code,
			CASE 
				WHEN code = ? THEN 1           -- Точное совпадение
				WHEN code LIKE ? THEN 2        -- Начинается с запроса
				WHEN code LIKE ? THEN 3        -- Содержит запрос
				ELSE 4
			END as relevance
		FROM genres 
		WHERE code LIKE ? OR code LIKE ?
		ORDER BY relevance, code
		LIMIT 100
	`, query, startWithQuery, likeQuery, startWithQuery, likeQuery)

	if err != nil {
		return nil, fmt.Errorf("ошибка поиска ID жанров: %v", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		var code string
		var relevance int
		if err := rows.Scan(&id, &code, &relevance); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// ==================== ПОЛУЧЕНИЕ ID КНИГ ПО ID СУЩНОСТЕЙ ====================

func (dm *Manager) findBookIDsByEntityIDs(entityIDs []int64, query string, errLabel string) ([]int64, error) {
	if len(entityIDs) == 0 {
		return []int64{}, nil
	}
	placeholders := strings.Repeat("?,", len(entityIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(entityIDs))
	for i, id := range entityIDs {
		args[i] = id
	}
	rows, err := dm.db.Query(fmt.Sprintf(query, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", errLabel, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (dm *Manager) findBookIDsByAuthorIDs(authorIDs []int64) ([]int64, error) {
	return dm.findBookIDsByEntityIDs(authorIDs, `
		SELECT DISTINCT b.id
		FROM books b
		JOIN book_authors ba ON b.id = ba.book_id
		WHERE ba.author_id IN (%s) AND b.del = 0
		ORDER BY b.added_at DESC
	`, "ошибка получения ID книг по авторам")
}

func (dm *Manager) findBookIDsBySeriesIDs(seriesIDs []int64) ([]int64, error) {
	return dm.findBookIDsByEntityIDs(seriesIDs, `
		SELECT id
		FROM books
		WHERE series_id IN (%s) AND del = 0
		ORDER BY series_no, title
	`, "ошибка получения ID книг по сериям")
}

func (dm *Manager) findBookIDsByGenreIDs(genreIDs []int64) ([]int64, error) {
	return dm.findBookIDsByEntityIDs(genreIDs, `
		SELECT DISTINCT b.id
		FROM books b
		JOIN book_genres bg ON b.id = bg.book_id
		WHERE bg.genre_id IN (%s) AND b.del = 0
		ORDER BY b.title
		LIMIT 1000
	`, "ошибка получения ID книг по жанрам")
}

// ==================== ПОИСК ПО НАЗВАНИЮ ====================

func (dm *Manager) searchBooksByTitle(title string) ([]domain.Book, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return []domain.Book{}, nil
	}
	pat := sqlLikeContains(title)
	rows, err := dm.db.Query(`
		SELECT id FROM books
		WHERE del = 0 AND title LIKE ? ESCAPE '\'
		ORDER BY added_at DESC
		LIMIT 1000`, pat)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска книг по названию: %v", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		ftsQuery := buildFtsQuery("title", title)
		if ftsQuery == "" {
			return []domain.Book{}, nil
		}

		ftsRows, err := dm.db.Query(`
			SELECT DISTINCT b.id
			FROM books b
			JOIN books_fts fts ON fts.rowid = b.id
			WHERE fts.title MATCH ? AND b.del = 0
			ORDER BY b.added_at DESC
			LIMIT 1000
		`, ftsQuery)

		if err != nil {
			return nil, fmt.Errorf("ошибка FTS поиска книг по названию: %v", err)
		}
		defer ftsRows.Close()

		for ftsRows.Next() {
			var id int64
			if err := ftsRows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return []domain.Book{}, nil
	}

	books, err := dm.GetBooksByIDs(ids)
	if err != nil {
		return nil, err
	}

	return sortBooksByTitlePriority(books, title), nil
}

// ==================== ПОЛУЧЕНИЕ ПОЛНЫХ ДАННЫХ КНИГ ПО ID ====================
// ИСПРАВЛЕННАЯ ВЕРСИЯ - убрано проблемное CASE выражение

func (dm *Manager) GetBooksByIDs(ids []int64) ([]domain.Book, error) {
	if len(ids) == 0 {
		return []domain.Book{}, nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	// ИСПРАВЛЕНИЕ: Убрано проблемное CASE выражение, используем простой IN
	query := fmt.Sprintf(`
		SELECT 
			b.id, 
			b.title, 
			s.name as series_name, 
			b.series_no, 
			b.file_name, 
			b.zip, 
			b.format, 
			b.file_size, 
			b.language, 
			b.added_at,
			(
				SELECT GROUP_CONCAT(a.name, ', ')
				FROM book_authors ba
				JOIN authors a ON ba.author_id = a.id
				WHERE ba.book_id = b.id
			) as authors,
			(
				SELECT GROUP_CONCAT(g.code, ', ')
				FROM book_genres bg
				JOIN genres g ON bg.genre_id = g.id
				WHERE bg.book_id = b.id
			) as genres
		FROM books b
		LEFT JOIN series s ON b.series_id = s.id
		WHERE b.id IN (%s)
	`, placeholders)

	rows, err := dm.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения книг по ID: %v", err)
	}
	defer rows.Close()

	var books []domain.Book
	for rows.Next() {
		var b domain.Book
		var seriesName sql.NullString
		var authorsStr, genresStr sql.NullString

		err := rows.Scan(
			&b.ID, &b.Title, &seriesName, &b.SeriesNo,
			&b.FileName, &b.Zip, &b.Format, &b.FileSize, &b.Language, &b.AddedAt,
			&authorsStr, &genresStr,
		)
		if err != nil {
			return nil, err
		}

		if seriesName.Valid {
			b.Series = seriesName.String
		}

		// Каждая книга получает ТОЛЬКО своих авторов
		if authorsStr.Valid {
			b.Author = authorsStr.String
			if b.Author != "" {
				b.AuthorsList = strings.Split(b.Author, ", ")
			}
		}

		if genresStr.Valid {
			b.Genre = genresStr.String
			if b.Genre != "" {
				b.GenresList = strings.Split(b.Genre, ", ")
			}
		}

		b.Format = bookutil.EnsureFormat(b.Format, b.FileName)
		books = append(books, b)
	}

	return books, nil
}

// ==================== РАСШИРЕННЫЙ ПОИСК (ДЛЯ API) ====================

// AdvancedSearchBooks - расширенный поиск с фильтрами
func (dm *Manager) AdvancedSearchBooks(filters domain.SearchFilters, page, perPage int) ([]domain.Book, int, error) {
	// Строим WHERE условия для поиска
	whereParts := []string{"b.del = 0"}
	args := []interface{}{}

	joins := []string{
		"LEFT JOIN series s ON b.series_id = s.id",
	}

	ftsJoins := []string{}
	ftsWhereParts := []string{}
	ftsArgs := []interface{}{}
	hasFTS := false

	// Добавляем условия поиска по каждому полю
	if filters.Title != "" {
		if q := buildFtsQuery("title", filters.Title); q != "" {
			ftsWhereParts = append(ftsWhereParts, "fts.title MATCH ?")
			ftsArgs = append(ftsArgs, q)
			hasFTS = true
		}
	}

	if filters.Series != "" {
		if q := buildFtsQuery("series", filters.Series); q != "" {
			ftsWhereParts = append(ftsWhereParts, "fts.series MATCH ?")
			ftsArgs = append(ftsArgs, q)
			hasFTS = true
		}
	}

	if hasFTS {
		ftsJoins = append(ftsJoins, "JOIN books_fts fts ON fts.rowid = b.id")
		whereParts = append(whereParts, strings.Join(ftsWhereParts, " AND "))
		args = append(args, ftsArgs...)
	}

	// Автор: точное имя — быстрый путь; иначе подстрока по Unicode lower в Go (findAuthorIDsBySearch).
	authorJoins := []string{}
	if filters.Author != "" {
		authorName := strings.TrimSpace(filters.Author)
		if authorName != "" {
			if authorID, ok := dm.authorIDByExactName(authorName); ok {
				authorJoins = append(authorJoins, "JOIN book_authors ba_ath_q ON b.id = ba_ath_q.book_id")
				whereParts = append(whereParts, "ba_ath_q.author_id = ?")
				args = append(args, authorID)
			} else {
				authorIDs, aerr := dm.findAuthorIDsBySearch(authorName)
				if aerr != nil {
					return nil, 0, aerr
				}
				if len(authorIDs) == 0 {
					whereParts = append(whereParts, "0")
				} else {
					if len(authorIDs) > 800 {
						authorIDs = authorIDs[:800]
					}
					ph := strings.Repeat("?,", len(authorIDs))
					ph = strings.TrimSuffix(ph, ",")
					authorJoins = append(authorJoins, "JOIN book_authors ba_ath_q ON b.id = ba_ath_q.book_id")
					whereParts = append(whereParts, fmt.Sprintf("ba_ath_q.author_id IN (%s)", ph))
					for _, aid := range authorIDs {
						args = append(args, aid)
					}
				}
			}
		}
	}

	allJoins := append(ftsJoins, append(authorJoins, joins...)...)

	// Фильтр по языку
	if filters.Language != "" && filters.Language != "Все языки" {
		whereParts = append(whereParts, "b.language = ?")
		args = append(args, filters.Language)
	}

	// Фильтр по жанру
	if filters.Genre != "" {
		terms := strings.Fields(filters.Genre)
		if len(terms) > 0 {
			// Добавляем JOIN для жанров только если есть фильтр по жанру
			allJoins = append(allJoins, 
				"JOIN book_genres bg ON b.id = bg.book_id",
				"JOIN genres g ON bg.genre_id = g.id",
			)
			
			placeholders := strings.Repeat("?,", len(terms))
			placeholders = strings.TrimSuffix(placeholders, ",")
			whereParts = append(whereParts, fmt.Sprintf("g.code IN (%s)", placeholders))
			for _, t := range terms {
				args = append(args, t)
			}
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	if whereClause == "" {
		whereClause = "1=1"
	}
	joinClause := strings.Join(allJoins, "\n")

	// Подсчёт общего количества
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT b.id)
		FROM books b
		%s
		WHERE %s`, joinClause, whereClause)

	var total int
	err := dm.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка выполнения count запроса: %v", err)
	}

	// Определяем сортировку
	orderClause := ""
	if filters.Title != "" {
		orderClause = "b.title ASC"
	} else if filters.Series != "" {
		orderClause = "b.series_no ASC, b.title ASC"
	} else if filters.Author != "" || filters.Genre != "" {
		orderClause = "b.title ASC"
	} else {
		orderClause = "b.added_at DESC"
	}

	// Получаем ID книг для текущей страницы
	idsQuery := fmt.Sprintf(`
		SELECT DISTINCT b.id
		FROM books b
		%s
		WHERE %s
		ORDER BY %s
		LIMIT ? OFFSET ?`, joinClause, whereClause, orderClause)

	idArgs := make([]interface{}, len(args))
	copy(idArgs, args)
	offset := (page - 1) * perPage
	idArgs = append(idArgs, perPage, offset)

	rows, err := dm.db.Query(idsQuery, idArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка получения ID книг: %v", err)
	}
	defer rows.Close()

	var bookIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		bookIDs = append(bookIDs, id)
	}

	if len(bookIDs) == 0 {
		return []domain.Book{}, total, nil
	}

	// Получаем полные данные книг по их ID
	books, err := dm.GetBooksByIDs(bookIDs)
	if err != nil {
		return nil, 0, err
	}

	// Применяем сортировку по релевантности если нужно
	if filters.Title != "" && filters.Author != "" {
		books = sortBooksByTitleAndAuthorPriority(books, filters.Title, filters.Author)
	} else if filters.Title != "" {
		books = sortBooksByTitlePriority(books, filters.Title)
	} else if filters.Author != "" {
		books = sortBooksByAuthorPriority(books, filters.Author)
	}

	return books, total, nil
}

// ==================== СУЩЕСТВУЮЩИЕ МЕТОДЫ (ОБНОВЛЕННЫЕ) ====================

func (dm *Manager) GetLatestBooks(limit int) ([]domain.Book, error) {
	rows, err := dm.db.Query(`
		SELECT id FROM books 
		WHERE del = 0 
		ORDER BY added_at DESC 
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return dm.GetBooksByIDs(ids)
}

func (dm *Manager) GetBookDownloadInfo(bookID int) (fileName, zipName, format, title, language, author string, del int, err error) {
	query := `
		SELECT b.file_name, b.zip, b.format, b.title, b.language, b.del
		FROM books b
		WHERE b.id = ?`

	err = dm.db.QueryRow(query, bookID).Scan(&fileName, &zipName, &format, &title, &language, &del)
	if err != nil {
		return "", "", "", "", "", "", 0, err
	}

	authorQuery := `
		SELECT GROUP_CONCAT(a.name, ', ')
		FROM book_authors ba
		JOIN authors a ON ba.author_id = a.id
		WHERE ba.book_id = ?`

	var authorStr sql.NullString
	err = dm.db.QueryRow(authorQuery, bookID).Scan(&authorStr)
	if err != nil && err != sql.ErrNoRows {
		return "", "", "", "", "", "", 0, err
	}

	if authorStr.Valid {
		author = authorStr.String
	}

	return fileName, zipName, format, title, language, author, del, nil
}

// ==================== МЕТАДАННЫЕ ====================

func (dm *Manager) GetInpxMtime() (float64, error) {
	var mtime float64
	err := dm.db.QueryRow("SELECT value FROM metadata WHERE key = 'inpx_mtime'").Scan(&mtime)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ошибка получения времени INPX: %v", err)
	}
	return mtime, nil
}

func (dm *Manager) UpdateInpxMtime(mtime float64) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	_, err := dm.db.Exec("INSERT INTO metadata (key, value) VALUES ('inpx_mtime', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", mtime)
	if err != nil {
		return fmt.Errorf("ошибка обновления времени INPX: %v", err)
	}
	return nil
}

func (dm *Manager) GetBookCount() (int64, error) {
	var count int64
	err := dm.db.QueryRow("SELECT COUNT(*) FROM books WHERE del = 0").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчета книг: %v", err)
	}
	return count, nil
}

func (dm *Manager) GetReadingProgress(userID int64, bookID int) (position string, ok bool, err error) {
	err = dm.db.QueryRow(
		`SELECT position FROM reading_progress WHERE user_id = ? AND book_id = ?`,
		userID, bookID,
	).Scan(&position)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return position, true, nil
}

func (dm *Manager) UpsertReadingProgress(userID int64, bookID int, position string) error {
	_, err := dm.db.Exec(
		`INSERT INTO reading_progress (user_id, book_id, position, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, book_id) DO UPDATE SET
			position = excluded.position,
			updated_at = CURRENT_TIMESTAMP`,
		userID, bookID, position,
	)
	return err
}

func (dm *Manager) GetGuestReadingProgress(guestKey string, bookID int) (position string, ok bool, err error) {
	err = dm.db.QueryRow(
		`SELECT position FROM reading_progress_guest WHERE guest_key = ? AND book_id = ?`,
		guestKey, bookID,
	).Scan(&position)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return position, true, nil
}

func (dm *Manager) UpsertGuestReadingProgress(guestKey string, bookID int, position string) error {
	_, err := dm.db.Exec(
		`INSERT INTO reading_progress_guest (guest_key, book_id, position, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(guest_key, book_id) DO UPDATE SET
			position = excluded.position,
			updated_at = CURRENT_TIMESTAMP`,
		guestKey, bookID, position,
	)
	return err
}

// DB exposes the underlying SQL connection for app-specific extensions (e.g. Liberama reader storage).
func (dm *Manager) DB() *sql.DB {
	return dm.db
}

func (dm *Manager) Close() error {
	return dm.db.Close()
}

// ==================== МЕТОДЫ ДЛЯ ПОЛУЧЕНИЯ СПИСКОВ ====================

// NameCount — имя сущности и число книг (для OPDS-навигации).
type NameCount struct {
	Name  string
	Count int
}

// GetAuthorsWithBookCounts возвращает авторов с числом книг одним запросом.
func (dm *Manager) GetAuthorsWithBookCounts(prefix string) ([]NameCount, error) {
	query := `
		SELECT a.name, COUNT(DISTINCT b.id)
		FROM authors a
		INNER JOIN book_authors ba ON ba.author_id = a.id
		INNER JOIN books b ON b.id = ba.book_id AND b.del = 0
		WHERE a.name LIKE ?
		GROUP BY a.id
		ORDER BY a.name
		LIMIT 100`
	rows, err := dm.db.Query(query, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []NameCount
	for rows.Next() {
		var nc NameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, err
		}
		res = append(res, nc)
	}
	return res, rows.Err()
}

// GetTitlesWithBookCounts возвращает названия с числом книг одним запросом.
func (dm *Manager) GetTitlesWithBookCounts(prefix string) ([]NameCount, error) {
	query := `
		SELECT title, COUNT(*)
		FROM books
		WHERE del = 0 AND title LIKE ?
		GROUP BY title
		ORDER BY title
		LIMIT 100`
	rows, err := dm.db.Query(query, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []NameCount
	for rows.Next() {
		var nc NameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, err
		}
		res = append(res, nc)
	}
	return res, rows.Err()
}

// GetSeriesWithBookCounts возвращает серии с числом книг одним запросом.
func (dm *Manager) GetSeriesWithBookCounts() ([]NameCount, error) {
	query := `
		SELECT s.name, COUNT(DISTINCT b.id)
		FROM series s
		INNER JOIN books b ON b.series_id = s.id AND b.del = 0
		GROUP BY s.id
		ORDER BY s.name
		LIMIT 1000`
	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []NameCount
	for rows.Next() {
		var nc NameCount
		if err := rows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, err
		}
		res = append(res, nc)
	}
	return res, rows.Err()
}

func (dm *Manager) authorIDByExactName(name string) (int64, bool) {
	var id int64
	err := dm.db.QueryRow(`SELECT id FROM authors WHERE name = ?`, name).Scan(&id)
	return id, err == nil
}

// CountMatchingBooks возвращает только число книг по фильтрам (без загрузки записей).
func (dm *Manager) CountMatchingBooks(filters domain.SearchFilters) (int, error) {
	_, total, err := dm.AdvancedSearchBooks(filters, 1, 1)
	return total, err
}

func (dm *Manager) GetAvailableLanguages() ([]string, error) {
	rows, err := dm.db.Query("SELECT DISTINCT language FROM books WHERE del = 0 ORDER BY language")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var langs []string
	for rows.Next() {
		var l string
		rows.Scan(&l)
		langs = append(langs, l)
	}
	return langs, nil
}

// ==================== МЕТОДЫ ДЛЯ ПОЛЬЗОВАТЕЛЕЙ ====================

func (dm *Manager) CreateUser(username, passwordHash, role string) error {
	_, err := dm.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		username, passwordHash, role)
	return err
}

func (dm *Manager) GetUserByUsername(username string) (id int64, passwordHash, role string, err error) {
	err = dm.db.QueryRow(`SELECT id, password_hash, role FROM users WHERE username = ?`, username).
		Scan(&id, &passwordHash, &role)
	return
}

func (dm *Manager) GetUserByID(id int64) (username, passwordHash, role string, err error) {
	err = dm.db.QueryRow(`SELECT username, password_hash, role FROM users WHERE id = ?`, id).
		Scan(&username, &passwordHash, &role)
	return
}

func (dm *Manager) UserCount() (int, error) {
	var n int
	err := dm.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (dm *Manager) ListUsers() ([]struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}, error) {
	rows, err := dm.db.Query(`SELECT id, username, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
	}
	for rows.Next() {
		var u struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			Role      string `json:"role"`
			CreatedAt string `json:"created_at"`
		}
		var created interface{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created); err != nil {
			return nil, err
		}
		if s, ok := created.(string); ok {
			u.CreatedAt = s
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func (dm *Manager) UpdateUserPassword(id int64, passwordHash string) error {
	_, err := dm.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

func (dm *Manager) UpdateUsername(id int64, username string) error {
	_, err := dm.db.Exec(`UPDATE users SET username = ? WHERE id = ?`, username, id)
	return err
}

func (dm *Manager) DeleteUser(id int64) (bool, error) {
	res, err := dm.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ==================== ИЗБРАННОЕ (КНИЖНАЯ ПОЛКА) ====================

func (dm *Manager) AddFavorite(userID int64, bookID int64) error {
	_, err := dm.db.Exec(`INSERT OR IGNORE INTO user_favorites (user_id, book_id) VALUES (?, ?)`, userID, bookID)
	return err
}

func (dm *Manager) RemoveFavorite(userID int64, bookID int64) error {
	_, err := dm.db.Exec(`DELETE FROM user_favorites WHERE user_id = ? AND book_id = ?`, userID, bookID)
	return err
}

func (dm *Manager) GetFavoriteBookIDs(userID int64) ([]int64, error) {
	rows, err := dm.db.Query(`SELECT book_id FROM user_favorites WHERE user_id = ? ORDER BY book_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (dm *Manager) IsFavorite(userID int64, bookID int64) (bool, error) {
	var n int
	err := dm.db.QueryRow(`SELECT 1 FROM user_favorites WHERE user_id = ? AND book_id = ?`, userID, bookID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ==================== СТАТИСТИКА БИБЛИОТЕКИ ====================

func (dm *Manager) GetLibraryStats() (totalBooks int64, totalAuthors int64, uniqueAuthors int64, genresCount int64, languagesCount int64, lastAddedAt *string, err error) {
	if err = dm.db.QueryRow(`SELECT COUNT(*) FROM books WHERE del = 0`).Scan(&totalBooks); err != nil {
		return
	}
	if err = dm.db.QueryRow(`SELECT COUNT(*) FROM book_authors`).Scan(&totalAuthors); err != nil {
		return
	}
	if err = dm.db.QueryRow(`SELECT COUNT(DISTINCT author_id) FROM book_authors`).Scan(&uniqueAuthors); err != nil {
		return
	}
	if err = dm.db.QueryRow(`SELECT COUNT(DISTINCT genre_id) FROM book_genres`).Scan(&genresCount); err != nil {
		return
	}
	if err = dm.db.QueryRow(`SELECT COUNT(DISTINCT language) FROM books WHERE del = 0`).Scan(&languagesCount); err != nil {
		return
	}
	var t sql.NullString
	if err = dm.db.QueryRow(`SELECT MAX(added_at) FROM books WHERE del = 0`).Scan(&t); err != nil {
		return
	}
	if t.Valid && t.String != "" {
		lastAddedAt = &t.String
	}
	err = nil
	return
}

// ==================== СОРТИРОВОЧНЫЕ ФУНКЦИИ ====================

func sortBooksByAuthorPriority(books []domain.Book, searchAuthor string) []domain.Book {
	searchAuthor = strings.Join(strings.Fields(searchAuthor), " ")
	searchVariants := generateAuthorSearchVariants(searchAuthor)
	searchTerms := strings.Fields(strings.ToLower(searchAuthor))

	sortedBooks := make([]domain.Book, len(books))
	copy(sortedBooks, books)

	sort.Slice(sortedBooks, func(i, j int) bool {
		bookI := sortedBooks[i]
		bookJ := sortedBooks[j]

		authorsI := strings.Split(strings.ReplaceAll(bookI.Author, ", ", ","), ",")
		authorsJ := strings.Split(strings.ReplaceAll(bookJ.Author, ", ", ","), ",")

		priorityI := getAuthorPriority(authorsI, searchTerms, searchVariants)
		priorityJ := getAuthorPriority(authorsJ, searchTerms, searchVariants)

		if priorityI != priorityJ {
			return priorityI < priorityJ
		}

		if len(authorsI) != len(authorsJ) {
			return len(authorsI) < len(authorsJ)
		}

		return titleLess(bookI, bookJ)
	})

	return sortedBooks
}

func getAuthorPriority(authors []string, searchTerms []string, searchVariants []string) int {
	for i, author := range authors {
		author = strings.TrimSpace(author)
		authorNorm := strings.ToLower(author)

		for _, variant := range searchVariants {
			if authorNorm == strings.ToLower(strings.TrimSpace(variant)) {
				if len(authors) == 1 {
					return 1
				} else if i == 0 {
					if len(authors) == 2 {
						return 2
					} else {
						return 3
					}
				} else {
					return 4
				}
			}
		}

		if containsAllTerms(authorNorm, searchTerms) {
			if len(authors) == 1 {
				return 1
			} else if i == 0 {
				if len(authors) == 2 {
					return 2
				} else {
					return 3
				}
			} else {
				return 4
			}
		}
	}

	return 5
}

func containsAllTerms(normStr string, terms []string) bool {
	for _, term := range terms {
		if term == "" {
			continue
		}
		if !strings.Contains(normStr, term) {
			return false
		}
	}
	return true
}

func normalizeSpacesLower(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func titleWords(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	words := make([]string, 0, 8)
	var b strings.Builder
	b.Grow(len(s))

	flush := func() {
		if b.Len() == 0 {
			return
		}
		words = append(words, strings.ToLower(b.String()))
		b.Reset()
	}

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return words
}

func isSingleWordQuery(q string) bool {
	return len(strings.Fields(q)) == 1
}

func containsWord(title string, word string) bool {
	word = normalizeSpacesLower(word)
	if word == "" {
		return false
	}
	tw := titleWords(title)
	for _, w := range tw {
		if w == word {
			return true
		}
	}
	return false
}

func containsAllWordsAnywhere(title string, query string) bool {
	qw := titleWords(query)
	if len(qw) == 0 {
		return false
	}
	tw := titleWords(title)
	if len(tw) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(tw))
	for _, w := range tw {
		set[w] = struct{}{}
	}
	for _, w := range qw {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

func containsPhraseAsWords(title string, query string) bool {
	qw := titleWords(query)
	if len(qw) == 0 {
		return false
	}
	tw := titleWords(title)
	if len(tw) < len(qw) {
		return false
	}
	for i := 0; i+len(qw) <= len(tw); i++ {
		match := true
		for j := 0; j < len(qw); j++ {
			if tw[i+j] != qw[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// GetTitlePriority ranks how well bookTitle matches searchTitle (lower is better).
func GetTitlePriority(bookTitle, searchTitle string) int {
	return getTitlePriority(bookTitle, searchTitle)
}

func getTitlePriority(bookTitle, searchTitle string) int {
	q := strings.TrimSpace(searchTitle)
	if q == "" {
		return 4
	}

	bt := strings.Join(titleWords(bookTitle), " ")
	qt := strings.Join(titleWords(q), " ")
	if bt != "" && bt == qt {
		return 0
	}

	if containsPhraseAsWords(bookTitle, q) {
		return 1
	}
	if containsAllWordsAnywhere(bookTitle, q) {
		return 2
	}

	titleLower := strings.ToLower(bookTitle)
	qLower := normalizeSpacesLower(q)
	if qLower != "" && strings.Contains(titleLower, qLower) {
		if isSingleWordQuery(q) && containsWord(bookTitle, qLower) {
			return 2
		}
		return 3
	}
	return 4
}

func isPreferredLanguage(lang string) int {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch l {
	case "ru", "rus", "ru-ru":
		return 0
	case "", "unknown":
		return 1
	default:
		return 2
	}
}

func hasPunctuationOrSymbols(s string) bool {
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return true
		}
	}
	return false
}

func titleSortClass(b domain.Book) int {
	langScore := isPreferredLanguage(b.Language)
	punctScore := 0
	if hasPunctuationOrSymbols(b.Title) {
		punctScore = 1
	}
	if langScore == 2 {
		return 100 + punctScore
	}
	return punctScore
}

func titleLess(a, b domain.Book) bool {
	ca, cb := titleSortClass(a), titleSortClass(b)
	if ca != cb {
		return ca < cb
	}
	na, nb := normalizeSpacesLower(a.Title), normalizeSpacesLower(b.Title)
	if na != nb {
		return na < nb
	}
	return a.Title < b.Title
}

func sortBooksByTitlePriority(books []domain.Book, searchTitle string) []domain.Book {
	if strings.TrimSpace(searchTitle) == "" {
		return books
	}
	sortedBooks := make([]domain.Book, len(books))
	copy(sortedBooks, books)
	sort.Slice(sortedBooks, func(i, j int) bool {
		pi := getTitlePriority(sortedBooks[i].Title, searchTitle)
		pj := getTitlePriority(sortedBooks[j].Title, searchTitle)
		if pi != pj {
			return pi < pj
		}
		return titleLess(sortedBooks[i], sortedBooks[j])
	})
	return sortedBooks
}

func sortBooksByTitleAndAuthorPriority(books []domain.Book, searchTitle, searchAuthor string) []domain.Book {
	sortedBooks := make([]domain.Book, len(books))
	copy(sortedBooks, books)

	searchAuthor = strings.Join(strings.Fields(searchAuthor), " ")
	searchVariants := generateAuthorSearchVariants(searchAuthor)
	searchTerms := strings.Fields(strings.ToLower(searchAuthor))

	sort.Slice(sortedBooks, func(i, j int) bool {
		bi, bj := sortedBooks[i], sortedBooks[j]
		tpi := getTitlePriority(bi.Title, searchTitle)
		tpj := getTitlePriority(bj.Title, searchTitle)
		if tpi != tpj {
			return tpi < tpj
		}

		authorsI := strings.Split(strings.ReplaceAll(bi.Author, ", ", ","), ",")
		authorsJ := strings.Split(strings.ReplaceAll(bj.Author, ", ", ","), ",")

		api := getAuthorPriority(authorsI, searchTerms, searchVariants)
		apj := getAuthorPriority(authorsJ, searchTerms, searchVariants)
		if api != apj {
			return api < apj
		}
		return titleLess(bi, bj)
	})
	return sortedBooks
}