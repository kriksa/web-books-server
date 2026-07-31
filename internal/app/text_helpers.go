package app

import (
	"strings"
)

func paragraphsFromPlainText(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := cleanText(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		if t := cleanText(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func htmlToParagraphs(html string) ([]string, error) {
	_, pars, _, _ := extractXhtmlText([]byte(html), "html-conv")
	if len(pars) > 0 {
		return pars, nil
	}
	return paragraphsFromPlainText(stripTags(html)), nil
}
