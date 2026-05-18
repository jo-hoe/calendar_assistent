package webcal

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

const (
	icsTimeLayout = "20060102T150405Z"
	icsDateLayout = "20060102"
	crlf          = "\r\n"
	maxLineLength = 75
)

type vevent struct {
	uid     string
	dtstart time.Time
	dtend   time.Time
	raw     []string // original property lines, excluding BEGIN/END
}

func parseICS(data []byte) ([]vevent, error) {
	lines := splitLines(string(data))
	var events []vevent
	var current []string
	inEvent := false

	for _, line := range lines {
		switch line {
		case "BEGIN:VEVENT":
			inEvent = true
			current = nil
		case "END:VEVENT":
			if inEvent {
				ev, err := buildVEvent(current)
				if err != nil {
					return nil, err
				}
				events = append(events, ev)
			}
			inEvent = false
		default:
			if inEvent {
				current = append(current, line)
			}
		}
	}
	return events, nil
}

func buildVEvent(lines []string) (vevent, error) {
	ev := vevent{raw: lines}
	for _, l := range lines {
		k, v := splitProp(l)
		switch {
		case k == "UID":
			ev.uid = v
		case k == "DTSTART" || strings.HasPrefix(k, "DTSTART;"):
			t, err := parseICSTime(v)
			if err != nil {
				return ev, fmt.Errorf("parsing DTSTART %q: %w", v, err)
			}
			ev.dtstart = t
		case k == "DTEND" || strings.HasPrefix(k, "DTEND;"):
			t, err := parseICSTime(v)
			if err != nil {
				return ev, fmt.Errorf("parsing DTEND %q: %w", v, err)
			}
			ev.dtend = t
		}
	}
	return ev, nil
}

func pruneExpired(events []vevent, ttl time.Duration) []vevent {
	cutoff := time.Now().UTC().Add(-ttl)
	out := events[:0]
	for _, ev := range events {
		if ev.dtend.IsZero() || ev.dtend.After(cutoff) {
			out = append(out, ev)
		}
	}
	return out
}

func newVEvent(event *llm.EventData) vevent {
	// Validate event times.
	endTime := event.EndTime
	if endTime.IsZero() || !endTime.After(event.StartTime) {
		endTime = event.StartTime.Add(time.Hour)
	}

	tz := event.TimeZone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	start := event.StartTime.In(loc)
	end := endTime.In(loc)

	titleSlug := sanitizeUID(event.Title)
	if titleSlug == "" {
		titleSlug = "event"
	}
	uid := fmt.Sprintf("%d-%s@calendar-assistent", event.StartTime.Unix(), titleSlug)
	now := time.Now().UTC().Format(icsTimeLayout)

	lines := []string{
		"UID:" + uid,
		"DTSTAMP:" + now,
		"DTSTART:" + start.UTC().Format(icsTimeLayout),
		"DTEND:" + end.UTC().Format(icsTimeLayout),
		"SUMMARY:" + escapeText(event.Title),
	}
	if event.Description != "" {
		lines = append(lines, "DESCRIPTION:"+escapeText(event.Description))
	}
	if event.Location != "" {
		lines = append(lines, "LOCATION:"+escapeText(event.Location))
	}

	return vevent{
		uid:     uid,
		dtstart: start.UTC(),
		dtend:   end.UTC(),
		raw:     lines,
	}
}

func serializeICS(events []vevent) []byte {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR" + crlf)
	sb.WriteString("VERSION:2.0" + crlf)
	sb.WriteString("PRODID:-//calendar-assistent//EN" + crlf)
	sb.WriteString("CALSCALE:GREGORIAN" + crlf)
	sb.WriteString("METHOD:PUBLISH" + crlf)

	for _, ev := range events {
		sb.WriteString("BEGIN:VEVENT" + crlf)
		for _, line := range ev.raw {
			sb.WriteString(foldLine(line) + crlf)
		}
		sb.WriteString("END:VEVENT" + crlf)
	}

	sb.WriteString("END:VCALENDAR" + crlf)
	return []byte(sb.String())
}

// splitLines handles both CRLF and LF line endings and unfolds continuation lines.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	raw := strings.Split(s, "\n")

	var lines []string
	for _, l := range raw {
		if l == "" {
			continue
		}
		if len(lines) > 0 && (l[0] == ' ' || l[0] == '\t') {
			lines[len(lines)-1] += strings.TrimLeft(l, " \t")
		} else {
			lines = append(lines, l)
		}
	}
	return lines
}

func splitProp(line string) (key, value string) {
	key, value, _ = strings.Cut(line, ":")
	return
}

func parseICSTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(icsTimeLayout, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("20060102T150405", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(icsDateLayout, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognised ICS time format %q", s)
}

func sanitizeUID(s string) string {
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

func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// foldLine wraps lines longer than maxLineLength octets per RFC 5545 §3.1.
// Split points are always on valid UTF-8 rune boundaries.
func foldLine(line string) string {
	b := []byte(line)
	if len(b) <= maxLineLength {
		return line
	}
	var sb strings.Builder

	// First line: up to maxLineLength bytes.
	end := maxLineLength
	for end > 0 && !utf8.RuneStart(b[end]) {
		end--
	}
	sb.Write(b[:end])
	b = b[end:]

	// Continuation lines: CRLF + space + up to (maxLineLength-1) bytes.
	for len(b) > 0 {
		sb.WriteString(crlf + " ")
		chunk := maxLineLength - 1
		if chunk >= len(b) {
			sb.Write(b)
			break
		}
		// Back up to a rune boundary.
		for chunk > 0 && !utf8.RuneStart(b[chunk]) {
			chunk--
		}
		sb.Write(b[:chunk])
		b = b[chunk:]
	}
	return sb.String()
}
