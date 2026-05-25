package webcal

import (
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/calendar/icsutil"
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
	folded := icsutil.FoldLine(long)
	for _, line := range strings.Split(strings.ReplaceAll(folded, "\r\n", "\n"), "\n") {
		if len(line) > icsutil.MaxLineLength {
			t.Errorf("line too long (%d): %q", len(line), line)
		}
	}
}

func TestFoldLine_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, got string)
	}{
		{
			name:  "empty string",
			input: "",
			check: func(t *testing.T, got string) {
				if got != "" {
					t.Errorf("FoldLine(%q) = %q, want empty", "", got)
				}
			},
		},
		{
			name:  "shorter than 75 bytes returned unchanged",
			input: "SUMMARY:Hello",
			check: func(t *testing.T, got string) {
				if got != "SUMMARY:Hello" {
					t.Errorf("FoldLine short string = %q, want unchanged", got)
				}
			},
		},
		{
			name:  "exactly 75 bytes no fold needed",
			input: strings.Repeat("a", icsutil.MaxLineLength),
			check: func(t *testing.T, got string) {
				if got != strings.Repeat("a", icsutil.MaxLineLength) {
					t.Errorf("FoldLine 75-byte string should be returned unchanged, got %q", got)
				}
				if strings.Contains(got, "\r\n") {
					t.Errorf("FoldLine 75-byte string should not contain CRLF")
				}
			},
		},
		{
			// 74 ASCII bytes then "é" (2-byte UTF-8); the rune starts at byte 74,
			// so the fold must not cut between its two bytes.
			name:  "multibyte UTF-8 at fold boundary not split",
			input: strings.Repeat("a", 74) + "é" + strings.Repeat("b", 10),
			check: func(t *testing.T, got string) {
				for i, line := range strings.Split(strings.ReplaceAll(got, "\r\n", "\n"), "\n") {
					if len(line) > icsutil.MaxLineLength {
						t.Errorf("segment %d exceeds MaxLineLength (%d): %q", i, len(line), line)
					}
				}
				reassembled := strings.ReplaceAll(got, "\r\n ", "")
				want := strings.Repeat("a", 74) + "é" + strings.Repeat("b", 10)
				if reassembled != want {
					t.Errorf("FoldLine round-trip mismatch: got %q, want %q", reassembled, want)
				}
			},
		},
		{
			name:  "200-byte line all segments within 75 bytes",
			input: strings.Repeat("x", 200),
			check: func(t *testing.T, got string) {
				for i, line := range strings.Split(strings.ReplaceAll(got, "\r\n", "\n"), "\n") {
					if len(line) > icsutil.MaxLineLength {
						t.Errorf("segment %d exceeds MaxLineLength (%d): %q", i, len(line), line)
					}
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := icsutil.FoldLine(tc.input)
			tc.check(t, got)
		})
	}
}
