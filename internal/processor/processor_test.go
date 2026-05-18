package processor

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type mockLLMClient struct {
	event *llm.EventData
	err   error
}

func (m *mockLLMClient) ExtractEvent(_ context.Context, _ io.Reader, _ llm.MIMEType) (*llm.EventData, error) {
	return m.event, m.err
}

type mockCalendar struct {
	id  string
	err error
}

func (m *mockCalendar) CreateEvent(_ context.Context, _ *llm.EventData) (string, error) {
	return m.id, m.err
}

func TestProcessor_ProcessArtifact(t *testing.T) {
	event := &llm.EventData{
		Title:     "Test",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		TimeZone:  "UTC",
	}

	p := New(&mockLLMClient{event: event}, &mockCalendar{id: "evt-123"})

	result, err := p.ProcessArtifact(context.Background(), strings.NewReader("test input"), llm.MIMEType("text/plain"))
	if err != nil {
		t.Fatalf("ProcessArtifact() error: %v", err)
	}

	if result.EventID != "evt-123" {
		t.Errorf("EventID = %q, want %q", result.EventID, "evt-123")
	}
	if result.EventData.Title != "Test" {
		t.Errorf("Title = %q, want %q", result.EventData.Title, "Test")
	}
}

func TestProcessor_ProcessArtifact_EmptyTitle(t *testing.T) {
	event := &llm.EventData{Title: ""}
	p := New(&mockLLMClient{event: event}, &mockCalendar{id: "x"})

	_, err := p.ProcessArtifact(context.Background(), strings.NewReader("test"), llm.MIMEType("text/plain"))
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}
