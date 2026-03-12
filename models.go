package main

import (
	"database/sql"
	"embed"
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	lru "github.com/hashicorp/golang-lru/v2"
)

//go:embed frontend/dist
var frontendFiles embed.FS

const (
	OPDSNS = "http://opds-spec.org/2010/catalog"
	AtomNS = "http://www.w3.org/2005/Atom"
	DCNs   = "http://purl.org/dc/terms/"
)

const (
	MaxBatchSize        = 5000 // Оптимально для M2M
	ItemsPerPage        = 50
	JWTKeyLength        = 32
	MaxMemoryBuffer     = 10 * 1024 * 1024
	DefaultBatchSize    = 5000
	CoverExtractTimeout = 10 * time.Second
)

var allowedSearchFields = map[string]bool{
	"author": true, "title": true, "series": true, "genre": true,
}

var supportedFormats = map[string]string{
	"fb2":  "application/fb2",
	"epub": "application/epub+zip",
	"mobi": "application/x-mobipocket-ebook",
	"pdf":  "application/pdf",
	"djvu": "image/vnd.djvu",
	"txt":  "text/plain",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"rtf":  "application/rtf",
	"html": "text/html",
	"htm":  "text/html",
}

var (
	errCoverNotFound   = errors.New("cover not found")
	missingCoverMarker = []byte{}
	imageCache         *lru.Cache[string, []byte]
)

func init() {
	var err error
	imageCache, err = lru.New[string, []byte](2000)
	if err != nil {
		panic(err)
	}
}

type ParseStatus struct {
	IsParsing            bool   `json:"is_parsing"`
	// Progress — текущий прогресс парсинга.
	// Единица измерения: байты распакованного текста INP (не строки).
	//
	// Почему байты:
	// - total можно заранее посчитать по ZIP-метаданным;
	// - не нужен отдельный проход для подсчёта строк;
	// - прогресс стабильнее на больших файлах.
	Progress             int64  `json:"progress"`
	// Total — общий объём данных для парсинга в тех же единицах, что и Progress (байты).
	Total                int64  `json:"total"`
	Message              string `json:"message"`
	StartTime            int64  `json:"start_time"`
	EstimatedRemainingSec int   `json:"estimated_remaining_sec"`
	Stage                string `json:"stage,omitempty"`
	CurrentFile          string `json:"current_file,omitempty"`
}

type SystemManager struct {
	Config      *Config
	DB          *DBManager
	Mu          sync.RWMutex
	ParseStatus ParseStatus
	StatusMu    sync.RWMutex
}

type OPDSFeed struct {
	XMLName   xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	XMLNSOPDS string      `xml:"xmlns:opds,attr,omitempty"`
	XMLNSDC   string      `xml:"xmlns:dc,attr,omitempty"`
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Updated   string      `xml:"updated"`
	Author    *OPDSAuthor `xml:"author,omitempty"`
	Links     []OPDSLink  `xml:"link"`
	Entries   []OPDSEntry `xml:"entry"`
}

type OPDSAuthor struct {
	Name string `xml:"name"`
}

type OPDSLink struct {
	Rel   string `xml:"rel,attr"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
}

type OPDSEntry struct {
	XMLName    xml.Name       `xml:"entry"`
	ID         string         `xml:"id"`
	Title      string         `xml:"title"`
	Updated    string         `xml:"updated"`
	Content    *OPDSText      `xml:"content,omitempty"`
	Links      []OPDSLink     `xml:"link"`
	Author     *OPDSAuthor    `xml:"author,omitempty"`
	Language   string         `xml:"dc:language,omitempty"`
	Identifier string         `xml:"dc:identifier,omitempty"`
	Category   []OPDSCategory `xml:"category,omitempty"`
}

type OPDSText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type OPDSCategory struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
}

type Config struct {
	BooksDir               string `json:"books_dir"`
	Port                   string `json:"port"`
	OPDSRoot               string `json:"opds_root"`
	AdminPasswordHash      string `json:"admin_password_hash,omitempty"`
	AdminUsername          string `json:"admin_username,omitempty"`
	JWTSigningKey          []byte `json:"jwt_signing_key,omitempty"`
	WebPasswordHash        string `json:"web_password_hash,omitempty"`
	ReaderEnabled          bool   `json:"reader_enabled,omitempty"`
	ReaderURL              string `json:"reader_url,omitempty"`
	DefaultSearchLanguage  string `json:"default_search_language,omitempty"` // код языка, напр. "ru"
}

type ConfigRequest struct {
	Config
	WebPassword string `json:"web_password"`
}

// Book обновлен для M2M
type Book struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Series    string    `json:"series"` // Имя серии для удобства
	SeriesID  int64     `json:"-"`
	SeriesNo  int       `json:"seriesNo"`
	FileName  string    `json:"fileName"`
	Zip       string    `json:"zip"`
	Format    string    `json:"format"`
	FileSize  int64     `json:"fileSize"`
	Language  string    `json:"language"`
	Del       int       `json:"-"`
	AddedAt   time.Time `json:"addedAt"`

	// Для совместимости с API (строка с разделителями)
	Author    string    `json:"author"`
	Genre     string    `json:"genre"`

	// Списки для нормализации
	AuthorsList []string `json:"authorsList,omitempty"`
	GenresList  []string `json:"genresList,omitempty"`

	Relevance float64   `json:"-"`
}

type SearchFilters struct {
	Author   string
	Title    string
	Series   string
	Genre    string
	Language string
}

type DBManager struct {
	db *sql.DB
	mu sync.RWMutex
}

type FieldIndices struct {
	Author, Title, Genre, Series, SeriesNo, File, Size, Del, Ext, Lang, Folder, Zip int
}

type Parser struct {
	config     *Config
	dbManager  *DBManager
	bookChan   chan Book
	wg         sync.WaitGroup
	stats      *Stats
	inpxInfo   InpxInfo
	indices    FieldIndices
	// onProgress сообщает прогресс парсинга в байтах (сколько распакованных байт INP уже обработано).
	// Важно: это не “строки”, а именно объём данных, чтобы:
	// - не делать второй проход для подсчёта строк;
	// - иметь стабильный прогресс даже на очень больших файлах.
	onProgress func(processedBytes int64)
	onTotal    func(total int64)
	onStage    func(stage, currentFile string)
}

type InpxInfo struct {
	Collection string
	Structure  []string
	Version    string
}

type Stats struct {
	// ProcessedBytes — суммарный прогресс по всем INP-файлам (в байтах распакованного текста).
	ProcessedBytes int64

	// ParsedBooks — сколько книг прошло валидацию и отправлено в очередь сохранения.
	ParsedBooks int64

	// SavedBooks — сколько книг реально записано в БД.
	SavedBooks int64
}

type FB2 struct {
	XMLName     xml.Name        `xml:"FictionBook"`
	Description FB2Description  `xml:"description"`
	Body        Body            `xml:"body"`
	Binary      []Binary        `xml:"binary"`
}

type FB2Description struct {
	TitleInfo    TitleInfo    `xml:"title-info"`
	SrcTitleInfo TitleInfo    `xml:"src-title-info"`
	PublishInfo  PublishInfo  `xml:"publish-info"`
	DocumentInfo DocumentInfo `xml:"document-info"`
}

type TitleInfo struct {
	Genre         []string       `xml:"genre"`
	Author        []FB2Author    `xml:"author"`
	BookTitle     string         `xml:"book-title"`
	Annotation    InnerXMLBuffer `xml:"annotation"`
	Keywords      string         `xml:"keywords"`
	Date          string         `xml:"date"`
	Coverpage     Coverpage      `xml:"coverpage"`
	Lang          string         `xml:"lang"`
	SrcLang       string         `xml:"src-lang"`
	Translator    []FB2Author    `xml:"translator"`
	Sequence      []FB2Sequence  `xml:"sequence"`
}

type PublishInfo struct {
	BookName  string        `xml:"book-name"`
	Publisher string        `xml:"publisher"`
	City      string        `xml:"city"`
	Year      string        `xml:"year"`
	ISBN      string        `xml:"isbn"`
	Sequence  []FB2Sequence `xml:"sequence"`
}

type DocumentInfo struct {
	Author      []FB2Author    `xml:"author"`
	ProgramUsed string         `xml:"program-used"`
	Date        string         `xml:"date"`
	SrcUrl      []string       `xml:"src-url"`
	SrcOcr      string         `xml:"src-ocr"`
	ID          string         `xml:"id"`
	Version     string         `xml:"version"`
	History     InnerXMLBuffer `xml:"history"`
	Publisher   []FB2Author    `xml:"publisher"`
}

type FB2Author struct {
	FirstName  string `xml:"first-name"`
	MiddleName string `xml:"middle-name"`
	LastName   string `xml:"last-name"`
	Nickname   string `xml:"nickname"`
	Email      string `xml:"email"`
}

func (a FB2Author) String() string {
	parts := []string{}
	if a.LastName != "" {
		parts = append(parts, a.LastName)
	}
	if a.FirstName != "" {
		parts = append(parts, a.FirstName)
	}
	if a.MiddleName != "" {
		parts = append(parts, a.MiddleName)
	}

	if len(parts) == 0 && a.Nickname != "" {
		return a.Nickname
	}
	return strings.Join(parts, " ")
}

type FB2Sequence struct {
	Name   string `xml:"name,attr"`
	Number int    `xml:"number,attr"`
	Lang   string `xml:"lang,attr"`
}

type InnerXMLBuffer struct {
	Content string `xml:",innerxml"`
}

type Coverpage struct {
	Image Image `xml:"image"`
}

type Image struct {
	Href      string `xml:"href,attr"`
	XLinkHref string `xml:"http://www.w3.org/1999/xlink href,attr"`
}

type Body struct {
	Title   string    `xml:"title"`
	Section []Section `xml:"section"`
}

type Section struct {
	Title     string   `xml:"title"`
	Paragraph []string `xml:"p"`
}

type Binary struct {
	Id          string `xml:"id,attr"`
	ContentType string `xml:"content-type,attr"`
	Data        string `xml:",chardata"`
}

type DetailedBookInfo struct {
	TitleInfo    map[string]interface{} `json:"titleInfo"`
	SrcTitleInfo map[string]interface{} `json:"srcTitleInfo"`
	PublishInfo  map[string]interface{} `json:"publishInfo"`
	DocumentInfo map[string]interface{} `json:"documentInfo"`
}

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthUser struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type PasswordChangeRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type UserCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserResetPasswordRequest struct {
	UserID      int64  `json:"user_id"`
	NewPassword string `json:"new_password"`
}

type UserDeleteRequest struct {
	UserID int64 `json:"user_id"`
}

type UserUpdateSelfRequest struct {
	OldPassword  string `json:"old_password"`
	NewPassword  string `json:"new_password"`
	NewUsername  string `json:"new_username,omitempty"`
}

type contextKey string

const claimsContextKey = contextKey("claims")

type customFileServer struct {
	root http.FileSystem
}

const defaultStructure = "author;genre;title;series;serno;file;size;libid;del;ext;date;insno;folder;lang;librate;keywords"

var configMu sync.Mutex

var (
	ErrInvalidSearchField = errors.New("invalid search field")
	ErrBookNotFound       = errors.New("book not found")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrDBNotReady         = errors.New("база данных недоступна или идет обновление")
)

type OpenSearchDescription struct {
	XMLName        xml.Name      `xml:"http://a9.com/-/spec/opensearch/1.1/ OpenSearchDescription"`
	XMLNS          string        `xml:"xmlns,attr"`
	ShortName      string        `xml:"ShortName"`
	Description    string        `xml:"Description"`
	InputEncoding  string        `xml:"InputEncoding"`
	OutputEncoding string        `xml:"OutputEncoding"`
	URL            OpenSearchURL `xml:"Url"`
}

type OpenSearchURL struct {
	Type     string `xml:"type,attr"`
	Template string `xml:"template,attr"`
}
