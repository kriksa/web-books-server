package opds

import "web_books/internal/domain"

// splitSeriesAndStandalone разделяет массив книг на:
// - seriesMap: книги, у которых book.Series != ""
// - standalone: книги без серии (book.Series == "")
//
// Используется в OPDS-хендлерах, чтобы убрать дублирование одинаковых циклов.
func splitSeriesAndStandalone(books []domain.Book) (seriesMap map[string][]domain.Book, standalone []domain.Book) {
	seriesMap = make(map[string][]domain.Book)
	standalone = make([]domain.Book, 0)

	for _, book := range books {
		if book.Series != "" {
			seriesMap[book.Series] = append(seriesMap[book.Series], book)
		} else {
			standalone = append(standalone, book)
		}
	}
	return seriesMap, standalone
}

