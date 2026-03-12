package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func NewParser(cfg *Config, dbManager *DBManager) *Parser {
	return &Parser{
		config:    cfg,
		dbManager: dbManager,
		bookChan:  make(chan Book, 10000),
		stats:     &Stats{},
		inpxInfo:  InpxInfo{},
		indices:   FieldIndices{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
	}
}

func (p *Parser) StartWorkers() {
	for i := 0; i < runtime.NumCPU(); i++ {
		p.wg.Add(1)
		go p.processBooks(i)
	}
}

func (p *Parser) processBooks(workerID int) {
	defer p.wg.Done()

	batch := make([]Book, 0, DefaultBatchSize)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
			case book, ok := <-p.bookChan:
				if !ok {
					if len(batch) > 0 {
						p.saveBatch(workerID, batch)
					}
					return
				}
				batch = append(batch, book)
				atomic.AddInt64(&p.stats.ParsedBooks, 1)

				if len(batch) >= DefaultBatchSize {
					p.saveBatch(workerID, batch)
					batch = batch[:0]
				}
			case <-ticker.C:
				if len(batch) > 0 {
					p.saveBatch(workerID, batch)
					batch = batch[:0]
				}
		}
	}
}

func (p *Parser) saveBatch(workerID int, batch []Book) {
	if err := p.dbManager.SaveBooksBatch(batch); err != nil {
		log.Printf("Рабочий %d: ошибка сохранения пакета: %v", workerID, err)
	}
	atomic.AddInt64(&p.stats.SavedBooks, int64(len(batch)))
}

func (p *Parser) mapStructure(structure []string) {
	p.indices = FieldIndices{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	for i, field := range structure {
		switch field {
			case "author":
				p.indices.Author = i
			case "title":
				p.indices.Title = i
			case "genre":
				p.indices.Genre = i
			case "series":
				p.indices.Series = i
			case "serno":
				p.indices.SeriesNo = i
			case "file":
				p.indices.File = i
			case "size":
				p.indices.Size = i
			case "del":
				p.indices.Del = i
			case "ext":
				p.indices.Ext = i
			case "lang":
				p.indices.Lang = i
			case "folder":
				p.indices.Folder = i
		}
	}
}

func (p *Parser) parseLine(line string, zipFiles map[string]bool) *Book {
	if line == "" {
		return nil
	}

	line = strings.ReplaceAll(line, "\x04", "|")
	if len(line) > 0 && line[len(line)-1] == '\x0D' {
		line = line[:len(line)-1]
	}

	parts := strings.Split(line, "|")
	book := &Book{AddedAt: time.Now(), Language: "unknown"}

	getVal := func(idx int) string {
		if idx >= 0 && idx < len(parts) {
			return strings.TrimSpace(parts[idx])
		}
		return ""
	}

	if idx := p.indices.Author; idx != -1 {
		book.AuthorsList = parseAuthors(getVal(idx))
	}
	if idx := p.indices.Title; idx != -1 {
		book.Title = getVal(idx)
	}
	if idx := p.indices.Genre; idx != -1 {
		raw := getVal(idx)
		if raw != "" {
			gs := strings.Split(raw, ":")
			for _, g := range gs {
				if g != "" {
					book.GenresList = append(book.GenresList, g)
				}
			}
		}
	}
	if idx := p.indices.Series; idx != -1 {
		book.Series = getVal(idx)
	}
	if idx := p.indices.SeriesNo; idx != -1 {
		if num, err := strconv.Atoi(getVal(idx)); err == nil {
			book.SeriesNo = num
		}
	}
	if idx := p.indices.File; idx != -1 {
		book.FileName = getVal(idx)
	}
	if idx := p.indices.Size; idx != -1 {
		if size, err := strconv.ParseInt(getVal(idx), 10, 64); err == nil {
			book.FileSize = size
		}
	}
	if idx := p.indices.Del; idx != -1 {
		if del, err := strconv.Atoi(getVal(idx)); err == nil {
			book.Del = del
		}
	}
	if idx := p.indices.Ext; idx != -1 {
		book.Format = strings.ToLower(getVal(idx))
	}
	if idx := p.indices.Lang; idx != -1 {
		if val := getVal(idx); val != "" {
			book.Language = val
		}
	}

	zipName := ""
	if idx := p.indices.Folder; idx != -1 {
		zipName = getVal(idx)
	}

	if zipName == "" {
		zipName = strings.ToLower(strings.TrimSuffix(filepath.Base(p.inpxInfo.Collection), ".inp")) + ".zip"
	}

	if _, ok := zipFiles[zipName]; ok {
		book.Zip = zipName
	} else {
		found := false
		for zipFile := range zipFiles {
			if strings.EqualFold(zipFile, zipName) {
				book.Zip = zipFile
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	if book.Del == 1 {
		return nil
	}

	if book.Title == "" || book.FileName == "" || book.Zip == "" || len(book.AuthorsList) == 0 {
		return nil
	}

	book.Format = ensureFormat(book.Format, book.FileName)
	return book
}

func parseAuthors(authorsStr string) []string {
	if authorsStr == "" {
		return []string{"Неизвестный автор"}
	}

	authors := strings.Split(authorsStr, ":")
	var formatted []string

	for _, author := range authors {
		author = strings.TrimSpace(author)
		if author == "" {
			continue
		}

		// Разделяем по запятым для обработки
		parts := strings.Split(author, ",")

		// Собираем обратно, но сохраняем порядок и все части
		var cleanParts []string
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				cleanParts = append(cleanParts, trimmed)
			}
		}

		if len(cleanParts) > 0 {
			// Собираем полное имя: фамилия + остальные части через пробел
			fullName := cleanParts[0]
			if len(cleanParts) > 1 {
				fullName += " " + strings.Join(cleanParts[1:], " ")
			}
			// Убираем множественные пробелы
			fullName = strings.Join(strings.Fields(fullName), " ")
			formatted = append(formatted, fullName)
		}
	}

	if len(formatted) == 0 {
		return []string{"Неизвестный автор"}
	}
	return formatted
}

func (p *Parser) ParseINPX(filePath string) error {
	if p.onStage != nil {
		p.onStage("init", "")
	}

	dir := filepath.Dir(filePath)
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ошибка чтения директории: %v", err)
	}

	zipFiles := make(map[string]bool)
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file.Name()), ".zip") {
			zipFiles[file.Name()] = true
		}
	}

	log.Printf("Найдено %d ZIP-файлов", len(zipFiles))

	r, err := zip.OpenReader(filePath)
	if err != nil {
		return fmt.Errorf("ошибка открытия INPX: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		switch f.Name {
			case "collection.info":
				if data, err := readZipFile(f); err == nil {
					p.inpxInfo.Collection = strings.TrimSpace(string(data))
				}
			case "structure.info":
				if data, err := readZipFile(f); err == nil {
					structure := strings.TrimSpace(string(data))
					if structure != "" {
						p.inpxInfo.Structure = strings.Split(strings.ToLower(structure), ";")
					}
				}
			case "version.info":
				if data, err := readZipFile(f); err == nil {
					p.inpxInfo.Version = strings.TrimSpace(string(data))
				}
		}
	}

	if len(p.inpxInfo.Structure) == 0 {
		log.Println("Файл structure.info не найден, используется структура по умолчанию")
		p.inpxInfo.Structure = strings.Split(defaultStructure, ";")
	}

	p.mapStructure(p.inpxInfo.Structure)
	log.Printf("Структура INPX применена: %v", p.inpxInfo.Structure)

	var inpFiles []*zip.File
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".inp") {
			inpFiles = append(inpFiles, f)
		}
	}

	log.Printf("Найдено %d INP-файлов", len(inpFiles))

	if p.onStage != nil {
		p.onStage("count", "")
	}

	// Быстрый подсчёт общего прогресса по распакованному размеру INP-файлов.
	//
	// Почему именно байты:
	// - размер известен сразу из ZIP-метаданных;
	// - не нужен второй проход по данным (как при подсчёте строк);
	// - прогресс хорошо масштабируется на больших коллекциях.
	var totalBytes int64
	for _, f := range inpFiles {
		if f.UncompressedSize64 > 0 {
			totalBytes += int64(f.UncompressedSize64)
		}
	}
	if p.onTotal != nil && totalBytes > 0 {
		p.onTotal(totalBytes)
	}

	if p.onStage != nil {
		p.onStage("parse", "")
	}

	inpChan := make(chan *zip.File, len(inpFiles))
	for _, inpFile := range inpFiles {
		inpChan <- inpFile
	}
	close(inpChan)

	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Защита от “падающих” воркеров: даже если где-то случится паника,
			// мы не хотим “уронить” весь процесс обновления базы.
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Паника в воркере парсинга INP: %v", r)
				}
			}()
			for inpFile := range inpChan {
				p.processINPFile(inpFile, zipFiles)
			}
		}()
	}
	wg.Wait()

	close(p.bookChan)
	p.wg.Wait()

	parsed := atomic.LoadInt64(&p.stats.ParsedBooks)
	saved := atomic.LoadInt64(&p.stats.SavedBooks)
	log.Printf("🏁 Парсинг завершен. Всего обработано: %d, Сохранено в БД: %d книг.", parsed, saved)

	return nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

func (p *Parser) processINPFile(inpFile *zip.File, zipFiles map[string]bool) {
	// Дополнительная страховка на уровне конкретного файла:
	// - ресурсы всё равно закроются (defer rc.Close());
	// - парсинг продолжится на следующих INP.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника при обработке INP-файла %s: %v", inpFile.Name, r)
		}
	}()

	if p.onStage != nil {
		p.onStage("parse_file", inpFile.Name)
	}

	rc, err := inpFile.Open()
	if err != nil {
		log.Printf("Ошибка открытия INP-файла %s: %v", inpFile.Name, err)
		return
	}
	defer rc.Close()

	// Потоковое чтение:
	// - читаем “до \n” через bufio.Reader;
	// - не накапливаем большие массивы строк;
	// - прогресс считаем по фактически прочитанным байтам распакованного INP.
	//
	// Важно: ReadSlice может вернуть bufio.ErrBufferFull для очень длинных строк —
	// тогда мы продолжаем накапливать lineBuf, пока не встретим '\n' или EOF.
	br := bufio.NewReaderSize(rc, 256*1024)

	var lineBuf []byte
	var lineNumber int64
	var localBytes int64
	var lastReported int64
	var lastAdded int64
	const reportEvery = int64(2 * 1024 * 1024) // обновление прогресса каждые ~2 МБ

	flushProgress := func(force bool) {
		if p.onProgress == nil {
			return
		}
		if force || (localBytes-lastReported) >= reportEvery {
			lastReported = localBytes
			// Дельта нужна, чтобы не прибавлять “localBytes” повторно на каждом репорте.
			// localBytes — счётчик внутри текущего INP-файла.
			delta := localBytes - lastAdded
			if delta < 0 {
				delta = 0
			}
			lastAdded = localBytes
			processed := atomic.AddInt64(&p.stats.ProcessedBytes, delta)
			p.onProgress(processed)
		}
	}

	for {
		chunk, err := br.ReadSlice('\n')
		if len(chunk) > 0 {
			localBytes += int64(len(chunk))
			lineBuf = append(lineBuf, chunk...)
			flushProgress(false)
		}

		if err == nil {
			// Полная строка
			lineNumber++
			line := strings.TrimRight(string(lineBuf), "\r\n")
			if book := p.parseLine(line, zipFiles); book != nil {
				p.bookChan <- *book
			}
			lineBuf = lineBuf[:0]
			continue
		}

		if err == bufio.ErrBufferFull {
			// Строка больше буфера — продолжаем накапливать.
			// Это редкий случай (обычно строки короткие), но важно не терять данные.
			continue
		}

		if err == io.EOF {
			// Последняя строка без '\n'
			if len(lineBuf) > 0 {
				lineNumber++
				line := strings.TrimRight(string(lineBuf), "\r\n")
				if book := p.parseLine(line, zipFiles); book != nil {
					p.bookChan <- *book
				}
				lineBuf = lineBuf[:0]
			}
			flushProgress(true)
			break
		}

		// Любая другая ошибка чтения
		log.Printf("Ошибка чтения INP-файла %s на строке %d: %v", inpFile.Name, lineNumber, err)
		flushProgress(true)
		break
	}
}
