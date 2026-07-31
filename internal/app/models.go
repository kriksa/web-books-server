package app

import (
	"net/http"
	"sync"

	"web_books/internal/domain"
)

const (
	MaxBatchSize        = domain.MaxBatchSize
	ItemsPerPage        = domain.ItemsPerPage
	JWTKeyLength        = domain.JWTKeyLength
	MaxMemoryBuffer     = domain.MaxMemoryBuffer
	DefaultBatchSize    = domain.DefaultBatchSize
	CoverExtractTimeout = domain.CoverExtractTimeout
	OPDSNS              = domain.OPDSNS
	AtomNS              = domain.AtomNS
	DCNs                = domain.DCNs
)

var (
	errCoverNotFound   = domain.ErrCoverNotFound
	missingCoverMarker = []byte{}
)

type ParseStatus = domain.ParseStatus

type SystemManager struct {
	Config      *Config
	DB          *DBManager
	Mu          sync.RWMutex
	ParseStatus ParseStatus
	StatusMu    sync.RWMutex
}

func (sm *SystemManager) PublicBaseURL() string {
	sm.Mu.RLock()
	defer sm.Mu.RUnlock()
	if sm.Config == nil {
		return ""
	}
	return sm.Config.PublicBaseURL
}

func (sm *SystemManager) BooksDir() string {
	sm.Mu.RLock()
	defer sm.Mu.RUnlock()
	if sm.Config == nil {
		return ""
	}
	return sm.Config.BooksDir
}

type Parser struct {
	config     *Config
	dbManager  *DBManager
	bookChan   chan Book
	wg         sync.WaitGroup
	stats      *Stats
	inpxInfo   InpxInfo
	indices    FieldIndices
	onProgress func(processedBytes int64)
	onTotal    func(total int64)
	onStage    func(stage, currentFile string)
}

type customFileServer struct {
	root http.FileSystem
}

const defaultStructure = domain.DefaultINPXStructure

var (
	ErrBookNotFound = domain.ErrBookNotFound
	ErrInvalidToken = domain.ErrInvalidToken
	ErrUnauthorized = domain.ErrUnauthorized
	ErrDBNotReady   = domain.ErrDBNotReady
)
