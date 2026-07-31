package opds

import (
	"fmt"
	"net/url"
	"sort"
	"time"

	"web_books/internal/domain"
)

type seriesEntrySpec struct {
	IDPrefix    string
	TitlePrefix string
	HrefFor     func(seriesName string) string
}

func buildSeriesSubsectionEntries(seriesMap map[string][]domain.Book, spec seriesEntrySpec) []domain.OPDSEntry {
	if len(seriesMap) == 0 {
		return nil
	}

	names := make([]string, 0, len(seriesMap))
	for name := range seriesMap {
		names = append(names, name)
	}
	sort.Strings(names)

	updated := time.Now().Format(time.RFC3339)
	entries := make([]domain.OPDSEntry, 0, len(names))

	for _, seriesName := range names {
		seriesBooks := seriesMap[seriesName]
		entries = append(entries, domain.OPDSEntry{
			ID:      fmt.Sprintf("urn:uuid:%s-%s", spec.IDPrefix, url.PathEscape(seriesName)),
			Title:   fmt.Sprintf("%s%s (%d книг)", spec.TitlePrefix, seriesName, len(seriesBooks)),
			Updated: updated,
			Links: []domain.OPDSLink{{
				Rel:  "subsection",
				Href: spec.HrefFor(seriesName),
				Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
			}},
		})
	}

	return entries
}

func buildStandaloneSubsectionEntry(id string, title string, href string) domain.OPDSEntry {
	return domain.OPDSEntry{
		ID:      id,
		Title:   title,
		Updated: time.Now().Format(time.RFC3339),
		Links: []domain.OPDSLink{{
			Rel:  "subsection",
			Href: href,
			Type: "application/atom+xml;profile=opds-catalog;kind=acquisition",
		}},
	}
}

