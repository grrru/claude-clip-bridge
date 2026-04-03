package bridge

import (
	"strings"
	"unicode"
)

func SanitizeSocketComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}

	var builder strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
		case r == '.', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "unknown"
	}

	return sanitized
}
