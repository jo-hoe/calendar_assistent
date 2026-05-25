package icsutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CRLF          = "\r\n"
	MaxLineLength = 75
)

func SanitizeUID(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func EscapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// FoldLine wraps lines longer than MaxLineLength octets per RFC 5545 §3.1.
// Split points are always on valid UTF-8 rune boundaries.
func FoldLine(line string) string {
	b := []byte(line)
	if len(b) <= MaxLineLength {
		return line
	}
	var sb strings.Builder

	end := MaxLineLength
	for end > 0 && !utf8.RuneStart(b[end]) {
		end--
	}
	sb.Write(b[:end])
	b = b[end:]

	for len(b) > 0 {
		sb.WriteString(CRLF + " ")
		chunk := MaxLineLength - 1
		if chunk >= len(b) {
			sb.Write(b)
			break
		}
		for chunk > 0 && !utf8.RuneStart(b[chunk]) {
			chunk--
		}
		sb.Write(b[:chunk])
		b = b[chunk:]
	}
	return sb.String()
}
