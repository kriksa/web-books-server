package app

import (
	"web_books/internal/config"
	"web_books/internal/domain"
	"web_books/internal/storage"
)

// Type aliases keep the core app package stable while types live in domain/storage.
type (
	Book            = domain.Book
	Config          = domain.Config
	ConfigRequest   = domain.ConfigRequest
	SearchFilters   = domain.SearchFilters
	Claims          = domain.Claims
	AuthRequest     = domain.AuthRequest
	AuthUser        = domain.AuthUser
	DetailedBookInfo = domain.DetailedBookInfo
	FB2             = domain.FB2
	FB2Author       = domain.FB2Author
	FB2Sequence     = domain.FB2Sequence
	Section         = domain.Section
	OPDSFeed        = domain.OPDSFeed
	OPDSEntry       = domain.OPDSEntry
	OPDSLink        = domain.OPDSLink
	OPDSAuthor      = domain.OPDSAuthor
	OPDSText        = domain.OPDSText
	OPDSCategory    = domain.OPDSCategory
	OpenSearchDescription = domain.OpenSearchDescription
	OpenSearchURL   = domain.OpenSearchURL
	FieldIndices    = domain.FieldIndices
	InpxInfo        = domain.InpxInfo
	Stats           = domain.Stats
	PasswordChangeRequest = domain.PasswordChangeRequest
	UserCreateRequest     = domain.UserCreateRequest
	UserResetPasswordRequest = domain.UserResetPasswordRequest
	UserDeleteRequest     = domain.UserDeleteRequest
	UserUpdateSelfRequest = domain.UserUpdateSelfRequest
)

type DBManager = storage.Manager

func NewDBManager(cfg *Config) (*DBManager, error) {
	return storage.New(cfg)
}

func LoadConfig() (*Config, error) {
	return config.Load()
}

func SaveConfig(cfg *Config) error {
	return config.Save(cfg)
}

func FindLatestINPX(cfg *Config) (string, error) {
	return config.FindLatestINPX(cfg)
}
