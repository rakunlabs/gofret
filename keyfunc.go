package gofret

import (
	"strings"
	"unicode"
)

// KeyFunc derives a map key from a Go field name. It is only consulted for
// fields whose struct tag does not spell out a name.
//
// See WithKeyFunc.
type KeyFunc func(string) string

// KeyNormalizer folds a key into a canonical form used for matching.
//
// Both the destination field names and the incoming keys are passed through
// the normalizer; two keys match when their normalized forms are equal. That
// makes matching precomputable, so lookups stay O(1) no matter how expensive
// the normalizer is.
//
// See WithKeyNormalizer.
type KeyNormalizer func(string) string

// FoldKey lowercases the key, which makes lookups case-insensitive while
// still telling separators apart. Pass it to WithKeyNormalizer to soften the
// default LooseKey matching without turning it off entirely.
func FoldKey(s string) string { return strings.ToLower(s) }

// LooseKey lowercases the key and removes '-', '_' and ' ', so "MaxRetry",
// "max_retry", "max-retry" and "max retry" all match. It is the default
// matcher.
func LooseKey(s string) string {
	var sb strings.Builder

	sb.Grow(len(s))

	for _, r := range s {
		switch r {
		case '-', '_', ' ':
			continue
		}

		sb.WriteRune(unicode.ToLower(r))
	}

	return sb.String()
}

// CamelCase renders the name in lowerCamelCase: "MaxRetry" becomes
// "maxRetry", "ID" becomes "id" and "HTTPServer" becomes "httpServer".
func CamelCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.Grow(len(s))
	sb.WriteString(strings.ToLower(words[0]))

	for _, w := range words[1:] {
		sb.WriteString(title(w))
	}

	return sb.String()
}

// PascalCase renders the name in UpperCamelCase: "max_retry" becomes
// "MaxRetry".
func PascalCase(s string) string {
	words := splitWords(s)

	var sb strings.Builder

	sb.Grow(len(s))

	for _, w := range words {
		sb.WriteString(title(w))
	}

	return sb.String()
}

// SnakeCase renders the name in snake_case: "MaxRetry" becomes "max_retry"
// and "HTTPServer" becomes "http_server".
func SnakeCase(s string) string { return joinWords(s, '_') }

// KebabCase renders the name in kebab-case: "MaxRetry" becomes "max-retry".
func KebabCase(s string) string { return joinWords(s, '-') }

// LowerCase lowercases the whole name without inserting separators.
func LowerCase(s string) string { return strings.ToLower(s) }

func joinWords(s string, sep byte) string {
	words := splitWords(s)

	var sb strings.Builder

	sb.Grow(len(s) + len(words))

	for i, w := range words {
		if i > 0 {
			sb.WriteByte(sep)
		}

		sb.WriteString(strings.ToLower(w))
	}

	return sb.String()
}

func title(s string) string {
	if s == "" {
		return ""
	}

	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])

	for i := 1; i < len(r); i++ {
		r[i] = unicode.ToLower(r[i])
	}

	return string(r)
}

// splitWords breaks an identifier into words on case boundaries, digit
// boundaries and the usual separators.
//
// A run of capitals is treated as one word, except that its final capital
// starts a new word when a lowercase letter follows, so "HTTPServer" splits
// into "HTTP" and "Server" rather than "HTTPS" and "erver".
func splitWords(s string) []string {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	words := make([]string, 0, 4)
	start := -1

	flush := func(end int) {
		if start >= 0 && end > start {
			words = append(words, string(runes[start:end]))
		}

		start = -1
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		switch {
		case r == '-' || r == '_' || r == ' ' || r == '.':
			flush(i)

			continue
		case start < 0:
			start = i

			continue
		}

		prev := runes[i-1]

		switch {
		// lower/digit -> upper starts a new word: "maxRetry".
		case unicode.IsUpper(r) && !unicode.IsUpper(prev):
			flush(i)
			start = i
		// upper -> upper followed by lower ends an acronym: "HTTPServer".
		case unicode.IsUpper(r) && unicode.IsUpper(prev) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1]):
			flush(i)
			start = i
		// letter -> digit and digit -> letter: "sha256Sum".
		case unicode.IsDigit(r) != unicode.IsDigit(prev):
			flush(i)
			start = i
		}
	}

	flush(len(runes))

	return words
}
