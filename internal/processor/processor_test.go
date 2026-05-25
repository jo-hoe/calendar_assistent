package processor

import (
	"context"
	"errors"
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

func TestProcessor_LLMError_WrapsErrCannotExtract(t *testing.T) {
	llmErr := errors.New("llm service unavailable")
	p := New(&mockLLMClient{err: llmErr}, &mockCalendar{id: "x"})

	_, err := p.ProcessArtifact(context.Background(), strings.NewReader("data"), llm.MIMEType("text/plain"))
	if err == nil {
		t.Fatal("expected error when llm returns error")
	}
	if !errors.Is(err, ErrCannotExtract) {
		t.Errorf("expected error to wrap ErrCannotExtract, got: %v", err)
	}
	if !errors.Is(err, llmErr) {
		t.Errorf("expected error to also wrap original llm error, got: %v", err)
	}
}

func TestProcessor_CalendarError_NotWrappedAsErrCannotExtract(t *testing.T) {
	event := &llm.EventData{
		Title:     "Meeting",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
	}
	calErr := errors.New("calendar backend down")
	p := New(&mockLLMClient{event: event}, &mockCalendar{err: calErr})

	_, err := p.ProcessArtifact(context.Background(), strings.NewReader("data"), llm.MIMEType("text/plain"))
	if err == nil {
		t.Fatal("expected error when calendar.CreateEvent returns error")
	}
	if errors.Is(err, ErrCannotExtract) {
		t.Errorf("calendar error should NOT be wrapped as ErrCannotExtract")
	}
	if !errors.Is(err, calErr) {
		t.Errorf("expected error to wrap original calendar error, got: %v", err)
	}
}
