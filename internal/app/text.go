package app
import (
	"html"
	"regexp"
	"strings"
)

var reStripTags = regexp.MustCompile(`(?is)<[^>]+>`)

func stripTags(s string) string {
	s = reStripTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return s
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmptyClean(vals ...string) string {
	for _, v := range vals {
		if s := cleanText(v); s != "" {
			return s
		}
	}
	return ""
}
