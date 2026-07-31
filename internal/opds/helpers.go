package opds

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"web_books/internal/domain"
)

const opdsAlphabet = "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯABCDEFGHIJKLMNOPQRSTUVWXYZ"

func opdsSearchLink(baseURL string) domain.OPDSLink {
	return domain.OPDSLink{
		Rel:   "search",
		Href:  baseURL + "/opds/opensearch",
		Type:  "application/opensearchdescription+xml",
		Title: "Поиск",
	}
}

// opdsSearchURLTemplate returns the OpenSearch URL template (not for Atom link hrefs).
// Path form avoids "Illegal character in query" in clients that parse {searchTerms} in query strings.
func opdsSearchURLTemplate(baseURL string) string {
	return baseURL + "/opds/search/q/{searchTerms}"
}

// searchQueryFromRequest reads the query from OPDS search URLs.
// Readers may send either ?term= (OpenSearch) or ?query= (some OPDS clients).
func searchQueryFromRequest(r *http.Request) string {
	if q := strings.TrimSpace(r.URL.Query().Get("term")); q != "" {
		return q
	}
	return strings.TrimSpace(r.URL.Query().Get("query"))
}

func booksWithoutSeries(books []domain.Book) []domain.Book {
	out := make([]domain.Book, 0, len(books))
	for _, book := range books {
		if book.Series == "" {
			out = append(out, book)
		}
	}
	return out
}

func opdsPrefixNavigationEntries(baseURL, pathPrefix, idPrefix, titleFmt string) []domain.OPDSEntry {
	var entries []domain.OPDSEntry
	now := time.Now().Format(time.RFC3339)
	for _, letter := range opdsAlphabet {
		letterStr := string(letter)
		entries = append(entries, domain.OPDSEntry{
			ID:      fmt.Sprintf("urn:uuid:%s-prefix-%s", idPrefix, letterStr),
			Title:   fmt.Sprintf(titleFmt, letterStr),
			Updated: now,
			Links: []domain.OPDSLink{{
				Rel:  "subsection",
				Href: baseURL + pathPrefix + url.QueryEscape(letterStr),
				Type: "application/atom+xml;profile=opds-catalog",
			}},
		})
	}
	return entries
}
