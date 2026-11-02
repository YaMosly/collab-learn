package models

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Titles use Unicode code points, not UTF-8 bytes or UTF-16 code units.
// Empty (including whitespace-only) resets the title to the UI fallback.
func NormalizeTitle(title string) (string, bool) {
	title = strings.TrimSpace(title)
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 100 {
		return "", false
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return title, true
}

type TitleWrite struct {
	BaseTitleRevision *int64  `json:"base_title_revision"`
	Title             *string `json:"title"`
}
