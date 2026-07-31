package httpapi

import "web_books/internal/storage"

// Host is the application facade required by catalog HTTP handlers.
type Host interface {
	ReloadServices(triggerParse bool) error
	BooksDir() string
}

// DBAccessor returns the library DB handle.
type DBAccessor func() *storage.Manager
