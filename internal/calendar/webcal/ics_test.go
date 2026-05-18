package webcal

import (
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

func TestParseAndSerializeRoundtrip(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:abc@test\r\nDTSTART:20260101T100000Z\r\nDTEND:20260101T110000Z\r\nSUMMARY:Test\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	events, err := parseICS([]byte(ics))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].uid != "abc@test" {
		t.Errorf("uid: got %q", events[0].uid)
	}
}

func TestPruneExpired(t *testing.T) {
	now := time.Now().UTC()
	old := vevent{uid: "old", dtend: now.Add(-48 * time.Hour)}
	fresh := vevent{uid: "fresh", dtend: now.Add(48 * time.Hour)}
	result := pruneExpired([]vevent{old, fresh}, 24*time.Hour)
	if len(result) != 1 || result[0].uid != "fresh" {
		t.Errorf("prune failed, got %v", result)
	}
}

func TestPruneKeepsWithinTTL(t *testing.T) {
	now := time.Now().UTC()
	// ended 10 hours ago — within 24h TTL, should be kept
	recent := vevent{uid: "recent", dtend: now.Add(-10 * time.Hour)}
	result := pruneExpired([]vevent{recent}, 24*time.Hour)
	if len(result) != 1 {
		t.Errorf("expected 1 event kept, got %d", len(result))
	}
}

func TestNewVEvent(t *testing.T) {
	ed := &llm.EventData{
		Title:     "Hello World",
		StartTime: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
		TimeZone:  "UTC",
	}
	ev := newVEvent(ed)
	if ev.uid == "" {
		t.Error("uid should not be empty")
	}
	found := false
	for _, l := range ev.raw {
		if strings.HasPrefix(l, "SUMMARY:") {
			found = true
			if !strings.Contains(l, "Hello World") {
				t.Errorf("summary line: %q", l)
			}
		}
	}
	if !found {
		t.Error("SUMMARY line missing")
	}
}

func TestSerializeICS(t *testing.T) {
	ev := newVEvent(&llm.EventData{
		Title:     "Test Event",
		StartTime: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
	})
	data := serializeICS([]vevent{ev})
	s := string(data)
	for _, want := range []string{"BEGIN:VCALENDAR", "BEGIN:VEVENT", "END:VEVENT", "END:VCALENDAR", "Test Event"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output", want)
		}
	}
	if !strings.Contains(s, "\r\n") {
		t.Error("missing CRLF line endings")
	}
}

func TestFoldLine(t *testing.T) {
	long := "DESCRIPTION:" + strings.Repeat("x", 100)
	folded := foldLine(long)
	for _, line := range strings.Split(strings.ReplaceAll(folded, "\r\n", "\n"), "\n") {
		if len(line) > maxLineLength {
			t.Errorf("line too long (%d): %q", len(line), line)
		}
	}
}
