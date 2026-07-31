package opds

import (
	"sync"

	"web_books/internal/domain"
)

// Catalog holds OPDS runtime settings (reserved for future use).
type Catalog struct {
	Config   *domain.Config
	ConfigFn func() *domain.Config
	Mu       sync.RWMutex
}
