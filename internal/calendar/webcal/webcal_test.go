package webcal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

func newTestProvider(ttl time.Duration) *webcalProvider {
	return &webcalProvider{
		store: &MockStorage{},
		ttl:   ttl,
	}
}

func TestCreateEvent_FirstRun(t *testing.T) {
	p := newTestProvider(720 * time.Hour)
	event := &llm.EventData{
		Title:     "First Event",
		StartTime: time.Now().Add(24 * time.Hour),
		EndTime:   time.Now().Add(25 * time.Hour),
	}
	id, err := p.CreateEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
	icsData, _ := p.store.(*MockStorage).Download(context.Background())
	if !strings.Contains(string(icsData), "First Event") {
		t.Error("ICS does not contain event title")
	}
}

func TestCreateEvent_AppendAndPrune(t *testing.T) {
	p := newTestProvider(24 * time.Hour)

	old := &llm.EventData{
		Title:     "Old Event",
		StartTime: time.Now().Add(-48 * time.Hour),
		EndTime:   time.Now().Add(-47 * time.Hour),
	}
	fresh := &llm.EventData{
		Title:     "Fresh Event",
		StartTime: time.Now().Add(24 * time.Hour),
		EndTime:   time.Now().Add(25 * time.Hour),
	}

	if _, err := p.CreateEvent(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CreateEvent(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}

	icsData, _ := p.store.(*MockStorage).Download(context.Background())
	s := string(icsData)
	if strings.Contains(s, "Old Event") {
		t.Error("old event should have been pruned")
	}
	if !strings.Contains(s, "Fresh Event") {
		t.Error("fresh event missing from ICS")
	}
}

func TestCreateEvent_MultipleEvents(t *testing.T) {
	p := newTestProvider(720 * time.Hour)

	for i := range 3 {
		ev := &llm.EventData{
			Title:     "Event",
			StartTime: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
			EndTime:   time.Now().Add(time.Duration(i+1)*24*time.Hour + time.Hour),
		}
		if _, err := p.CreateEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	icsData, _ := p.store.(*MockStorage).Download(context.Background())
	events, err := parseICS(icsData)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}
