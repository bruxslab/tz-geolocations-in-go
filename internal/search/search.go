// Package search constructs PostgreSQL-safe search inputs with legacy sanitization.
package search

import (
	"strings"
	"unicode/utf16"
)

func UTF16Length(s string) int { return len(utf16.Encode([]rune(s))) }
func whitespace(r rune) bool {
	return r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r' || r == ' ' || r == '\u00a0' || r == '\u1680' || r >= '\u2000' && r <= '\u200a' || r == '\u2028' || r == '\u2029' || r == '\u202f' || r == '\u205f' || r == '\u3000' || r == '\ufeff'
}
func Sanitize(q string) string {
	return strings.TrimFunc(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || whitespace(r) {
			return r
		}
		return -1
	}, q), whitespace)
}

// Prefix applies :* only to the final token, matching FTS5 phrase-prefix intent.
// Tokens are ASCII alphanumeric after sanitization, so no tsquery operators survive.
func Prefix(q string) string {
	words := strings.FieldsFunc(q, whitespace)
	if len(words) == 0 {
		return ""
	}
	for i := range words {
		words[i] = "'" + words[i] + "'"
	}
	words[len(words)-1] += ":*"
	return strings.Join(words, " <-> ")
}
