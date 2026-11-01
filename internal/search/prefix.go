// Package search turns free-text queries into word-prefix terms for the
// SQLite FTS5 and PostgreSQL tsvector item-search paths. Both engines use a
// language-neutral tokenizer, so a term matches any word that starts with the
// typed text ("Outlo" matches "Outlook").
package search

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// minTokenLen is the shortest term sent to the index. A one-character
	// prefix matches too large a share of the index to be worth an FTS probe,
	// so a query with no term this long falls back to LIKE.
	minTokenLen = 2
	// maxTokens bounds the generated expression so pathological input cannot
	// build an unbounded tsquery or MATCH string.
	maxTokens = 8
)

// Tokens splits raw into lowercase word terms for prefix matching. It returns
// nil when the query has no term of at least minTokenLen runes, or contains a
// script (CJK, Thai) that neither engine segments into words; callers then use
// a LIKE fallback. Terms contain only letters and digits.
func Tokens(raw string) []string {
	if raw == "" {
		return nil
	}
	var (
		tokens []string
		b      strings.Builder
	)
	flush := func() {
		if b.Len() == 0 {
			return
		}
		token := b.String()
		b.Reset()
		if utf8.RuneCountInString(token) >= minTokenLen {
			tokens = append(tokens, token)
		}
	}
	for _, r := range raw {
		if isUnsegmentable(r) {
			return nil
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
		if len(tokens) >= maxTokens {
			return tokens[:maxTokens]
		}
	}
	flush()
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}
	return tokens
}

// SQLiteMatch renders terms as an FTS5 MATCH expression. Adjacent terms are
// implicitly ANDed and each term is a prefix query.
func SQLiteMatch(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = token + "*"
	}
	return strings.Join(parts, " ")
}

// PostgresTSQuery renders terms as a to_tsquery expression. Every term must
// match as a prefix and the terms are ANDed. Tokens only contain letters and
// digits, so the generated expression cannot carry tsquery operators.
func PostgresTSQuery(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = token + ":*"
	}
	return strings.Join(parts, " & ")
}

// isUnsegmentable reports whether r belongs to a script that neither the FTS5
// unicode61 tokenizer nor the PostgreSQL default parser splits into words. Such
// queries stay on the LIKE path until CJK segmentation is added.
func isUnsegmentable(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x9FFF, // CJK radicals through unified ideographs
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0x3040 && r <= 0x30FF, // Hiragana and Katakana
		r >= 0xAC00 && r <= 0xD7AF, // Hangul syllables
		r >= 0x0E00 && r <= 0x0E7F: // Thai
		return true
	}
	return false
}
