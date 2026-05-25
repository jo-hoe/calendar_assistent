package smtp

import (
	"fmt"
	"strings"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/calendar/icsutil"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

const (
	icsTimeLayout    = "20060102T150405Z"
	mailtoPrefix     = "mailto:"
	rsvpAttendeeFlag = "RSVP=TRUE"
)

// buildICS constructs a METHOD:REQUEST iCalendar for a single event.
// organizer is the ORGANIZER value (e.g. "mailto:sender@example.com").
// attendee is the ATTENDEE value (e.g. "mailto:recipient@example.com").
func buildICS(event *llm.EventData, organizer, attendee string) []byte {
	var sb strings.Builder
	writeCalendarHeader(&sb)
	writeEventBlock(&sb, event, organizer, attendee)
	sb.WriteString("END:VCALENDAR" + icsutil.CRLF)
	return []byte(sb.String())
}

func writeCalendarHeader(sb *strings.Builder) {
	sb.WriteString("BEGIN:VCALENDAR" + icsutil.CRLF)
	sb.WriteString("VERSION:2.0" + icsutil.CRLF)
	sb.WriteString("PRODID:-//calendar-assistent//EN" + icsutil.CRLF)
	sb.WriteString("CALSCALE:GREGORIAN" + icsutil.CRLF)
	sb.WriteString("METHOD:REQUEST" + icsutil.CRLF)
}

func writeEventBlock(sb *strings.Builder, event *llm.EventData, organizer, attendee string) {
	sb.WriteString("BEGIN:VEVENT" + icsutil.CRLF)
	for _, line := range buildEventLines(event, organizer, attendee) {
		sb.WriteString(icsutil.FoldLine(line) + icsutil.CRLF)
	}
	sb.WriteString("END:VEVENT" + icsutil.CRLF)
}

func buildEventLines(event *llm.EventData, organizer, attendee string) []string {
	now := time.Now().UTC().Format(icsTimeLayout)
	uid := buildUID(event)
	start := event.StartTime.UTC().Format(icsTimeLayout)
	end := resolveEndTime(event).UTC().Format(icsTimeLayout)

	lines := []string{
		"UID:" + uid,
		"DTSTAMP:" + now,
		"DTSTART:" + start,
		"DTEND:" + end,
		"ORGANIZER:" + ensureMailto(organizer),
		"ATTENDEE;" + rsvpAttendeeFlag + ":" + ensureMailto(attendee),
		"SUMMARY:" + icsutil.EscapeText(event.Title),
	}
	return appendOptionalFields(lines, event)
}

func appendOptionalFields(lines []string, event *llm.EventData) []string {
	if event.Description != "" {
		lines = append(lines, "DESCRIPTION:"+icsutil.EscapeText(event.Description))
	}
	if event.Location != "" {
		lines = append(lines, "LOCATION:"+icsutil.EscapeText(event.Location))
	}
	return lines
}

func buildUID(event *llm.EventData) string {
	slug := icsutil.SanitizeUID(event.Title)
	if slug == "" {
		slug = "event"
	}
	return fmt.Sprintf("%d-%s@calendar-assistent", event.StartTime.Unix(), slug)
}

func resolveEndTime(event *llm.EventData) time.Time {
	if !event.EndTime.IsZero() && event.EndTime.After(event.StartTime) {
		return event.EndTime
	}
	return event.StartTime.Add(time.Hour)
}

// ensureMailto prepends "mailto:" if the value does not already start with it.
func ensureMailto(s string) string {
	if strings.HasPrefix(s, mailtoPrefix) {
		return s
	}
	return mailtoPrefix + s
}
