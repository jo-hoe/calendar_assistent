package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
	"github.com/jo-hoe/calendar-assistent/internal/processor"
)

type mockLLM struct {
	event *llm.EventData
	err   error
}

func (m *mockLLM) ExtractEvent(_ context.Context, _ io.Reader, _ llm.MIMEType) (*llm.EventData, error) {
	return m.event, m.err
}

type mockCalendar struct {
	id  string
	err error
}

func (m *mockCalendar) CreateEvent(_ context.Context, _ *llm.EventData) (string, error) {
	return m.id, m.err
}

func newTestServer(apiKey string, llmClient *mockLLM, calClient *mockCalendar) *Server {
	proc := processor.New(llmClient, calClient)
	cfg := config.ServerConfig{
		Address:      ":0",
		ReadTimeout:  config.Duration{Duration: 10 * time.Second},
		WriteTimeout: config.Duration{Duration: 10 * time.Second},
		IdleTimeout:  config.Duration{Duration: 10 * time.Second},
		MaxUpload:    10 * 1024 * 1024,
		APIKey:       apiKey,
	}
	return New(cfg, proc, slog.Default())
}

func TestHealthz(t *testing.T) {
	s := newTestServer("", &mockLLM{}, &mockCalendar{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleText_Success(t *testing.T) {
	event := &llm.EventData{
		Title:     "Meeting",
		StartTime: time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		TimeZone:  "UTC",
	}

	s := newTestServer("", &mockLLM{event: event}, &mockCalendar{id: "cal-123"})

	body := `{"text":"Meeting tomorrow at 2pm"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var result processor.Result
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result.EventID != "cal-123" {
		t.Errorf("EventID = %q, want %q", result.EventID, "cal-123")
	}
}

func TestHandleText_EmptyText(t *testing.T) {
	s := newTestServer("", &mockLLM{}, &mockCalendar{})

	body := `{"text":""}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleArtifact_Success(t *testing.T) {
	event := &llm.EventData{
		Title:     "Concert",
		StartTime: time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 6, 1, 23, 0, 0, 0, time.UTC),
		TimeZone:  "UTC",
	}

	s := newTestServer("", &mockLLM{event: event}, &mockCalendar{id: "evt-456"})

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "ticket.png")
	_, _ = part.Write([]byte("fake-png-data"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/events/artifact", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestHandleArtifact_UnsupportedType(t *testing.T) {
	s := newTestServer("", &mockLLM{}, &mockCalendar{})

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "data.zip")
	_, _ = part.Write([]byte("zip-data"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/events/artifact", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAPIKeyAuth_Required(t *testing.T) {
	s := newTestServer("secret-key", &mockLLM{}, &mockCalendar{})

	body := `{"text":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	event := &llm.EventData{
		Title:     "Test",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
		TimeZone:  "UTC",
	}
	s := newTestServer("secret-key", &mockLLM{event: event}, &mockCalendar{id: "x"})

	body := `{"text":"meeting tomorrow"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret-key")
	w := httptest.NewRecorder()

	s.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		filename string
		want     llm.MIMEType
	}{
		{"photo.png", llm.MIMEType("image/png")},
		{"doc.PDF", llm.MIMEType("application/pdf")},
		{"file.jpg", llm.MIMEType("image/jpeg")},
		{"file.jpeg", llm.MIMEType("image/jpeg")},
		{"page.html", llm.MIMEType("text/html")},
		{"notes.txt", llm.MIMEType("text/plain")},
		{"unknown.xyz", llm.MIMEType("application/octet-stream")},
	}
	for _, tc := range tests {
		got := detectMIMEType(tc.filename)
		if got != tc.want {
			t.Errorf("detectMIMEType(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}
