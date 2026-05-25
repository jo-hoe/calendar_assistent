package smtp

import (
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

func testEvent() *llm.EventData {
	return &llm.EventData{
		Title:       "Team Sync",
		Description: "Weekly team sync",
		Location:    "Conference Room A",
		StartTime:   time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
	}
}

func TestBuildICS_ContainsMethodRequest(t *testing.T) {
	out := string(buildICS(testEvent(), "mailto:org@example.com", "mailto:att@example.com"))
	if !strings.Contains(out, "METHOD:REQUEST") {
		t.Errorf("expected METHOD:REQUEST in output, got:\n%s", out)
	}
}

func TestBuildICS_ContainsOrganizer(t *testing.T) {
	out := string(buildICS(testEvent(), "mailto:org@example.com", "mailto:att@example.com"))
	if !strings.Contains(out, "ORGANIZER:") {
		t.Errorf("expected ORGANIZER: in output, got:\n%s", out)
	}
}

func TestBuildICS_ContainsAttendeeWithRSVP(t *testing.T) {
	out := string(buildICS(testEvent(), "mailto:org@example.com", "mailto:att@example.com"))
	if !strings.Contains(out, "ATTENDEE;RSVP=TRUE:") {
		t.Errorf("expected ATTENDEE;RSVP=TRUE: in output, got:\n%s", out)
	}
}

func TestBuildICS_ContainsVEVENTBoundaries(t *testing.T) {
	out := string(buildICS(testEvent(), "mailto:org@example.com", "mailto:att@example.com"))
	if !strings.Contains(out, "BEGIN:VEVENT") {
		t.Errorf("expected BEGIN:VEVENT in output")
	}
	if !strings.Contains(out, "END:VEVENT") {
		t.Errorf("expected END:VEVENT in output")
	}
}

func TestBuildICS_UsesCRLFLineEndings(t *testing.T) {
	out := buildICS(testEvent(), "mailto:org@example.com", "mailto:att@example.com")
	s := string(out)
	// Every \n must be preceded by \r
	for i, ch := range s {
		if ch == '\n' && (i == 0 || s[i-1] != '\r') {
			t.Errorf("found bare \\n at position %d (no preceding \\r)", i)
		}
	}
}

func TestBuildICS_OrganizerMailtoPrepended(t *testing.T) {
	// Pass organizer without "mailto:" prefix — should be auto-prepended.
	out := string(buildICS(testEvent(), "org@example.com", "mailto:att@example.com"))
	if !strings.Contains(out, "ORGANIZER:mailto:org@example.com") {
		t.Errorf("expected ORGANIZER:mailto:org@example.com in output, got:\n%s", out)
	}
}

func TestBuildICS_AttendeeMailtoPrepended(t *testing.T) {
	out := string(buildICS(testEvent(), "mailto:org@example.com", "att@example.com"))
	if !strings.Contains(out, "ATTENDEE;RSVP=TRUE:mailto:att@example.com") {
		t.Errorf("expected ATTENDEE;RSVP=TRUE:mailto:att@example.com in output, got:\n%s", out)
	}
}

func TestBuildICS_NoDescriptionOrLocation(t *testing.T) {
	ev := &llm.EventData{
		Title:     "Minimal",
		StartTime: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
	}
	out := string(buildICS(ev, "mailto:org@example.com", "mailto:att@example.com"))
	if strings.Contains(out, "DESCRIPTION:") {
		t.Errorf("did not expect DESCRIPTION when description is empty")
	}
	if strings.Contains(out, "LOCATION:") {
		t.Errorf("did not expect LOCATION when location is empty")
	}
}
