package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
)

func TestMockClient_ExtractEvent(t *testing.T) {
	client, err := NewClient(config.LLMConfig{Provider: "mock"})
	if err != nil {
		t.Fatalf("NewClient(mock) error: %v", err)
	}

	event, err := client.ExtractEvent(context.Background(), strings.NewReader("test"), MIMEType("text/plain"))
	if err != nil {
		t.Fatalf("ExtractEvent() error: %v", err)
	}

	if event.Title != "Mock Event" {
		t.Errorf("Title = %q, want %q", event.Title, "Mock Event")
	}
	if event.Location != "Mock Location" {
		t.Errorf("Location = %q, want %q", event.Location, "Mock Location")
	}
}

func TestAIProxyClient_ExtractEvent(t *testing.T) {
	eventData := EventData{
		Title:       "Test Meeting",
		Description: "A test meeting",
		StartTime:   time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Location:    "Room 1",
		TimeZone:    "UTC",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		eventJSON, _ := json.Marshal(eventData)
		resp := chatCompletionResponse{
			Choices: []chatChoice{
				{Message: responseMsg{Content: string(eventJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(config.LLMConfig{
		Provider: "aiproxy",
		AIProxy: config.AIProxyConfig{
			BaseURL:     server.URL,
			APIKey:      "test-key",
			Model:       "test-model",
			Temperature: 0.2,
			MaxTokens:   1024,
		},
	})
	if err != nil {
		t.Fatalf("NewClient(aiproxy) error: %v", err)
	}

	event, err := client.ExtractEvent(context.Background(), strings.NewReader("Meeting on March 15 at 2pm"), MIMEType("text/plain"))
	if err != nil {
		t.Fatalf("ExtractEvent() error: %v", err)
	}

	if event.Title != "Test Meeting" {
		t.Errorf("Title = %q, want %q", event.Title, "Test Meeting")
	}
	if event.Location != "Room 1" {
		t.Errorf("Location = %q, want %q", event.Location, "Room 1")
	}
}

func TestAIProxyClient_ExtractEvent_WithCodeFence(t *testing.T) {
	eventData := EventData{
		Title:     "Fenced Event",
		StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
		TimeZone:  "UTC",
	}

	eventJSON, _ := json.Marshal(eventData)
	fencedResponse := "```json\n" + string(eventJSON) + "\n```"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := chatCompletionResponse{
			Choices: []chatChoice{
				{Message: responseMsg{Content: fencedResponse}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(config.LLMConfig{
		Provider: "aiproxy",
		AIProxy: config.AIProxyConfig{
			BaseURL:     server.URL,
			APIKey:      "key",
			Model:       "m",
			Temperature: 0.2,
			MaxTokens:   1024,
		},
	})
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	event, err := client.ExtractEvent(context.Background(), strings.NewReader("test"), MIMEType("text/plain"))
	if err != nil {
		t.Fatalf("ExtractEvent() error: %v", err)
	}
	if event.Title != "Fenced Event" {
		t.Errorf("Title = %q, want %q", event.Title, "Fenced Event")
	}
}

func TestAIProxyClient_ExtractEvent_ImageMimeType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if len(req.Messages) < 2 {
			t.Fatal("expected at least 2 messages")
		}
		userMsg := req.Messages[1]
		if len(userMsg.Content) < 2 {
			t.Fatal("expected at least 2 content parts in user message")
		}
		if userMsg.Content[0].Type != partTypeImageURL {
			t.Errorf("first part type = %q, want image_url", userMsg.Content[0].Type)
		}
		if !strings.HasPrefix(userMsg.Content[0].ImageURL.URL, "data:image/png;base64,") {
			t.Errorf("unexpected data URL prefix: %s", userMsg.Content[0].ImageURL.URL[:30])
		}

		eventJSON, _ := json.Marshal(EventData{
			Title:     "Image Event",
			StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
			TimeZone:  "UTC",
		})
		resp := chatCompletionResponse{
			Choices: []chatChoice{{Message: responseMsg{Content: string(eventJSON)}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(config.LLMConfig{
		Provider: "aiproxy",
		AIProxy: config.AIProxyConfig{
			BaseURL:     server.URL,
			APIKey:      "key",
			Model:       "m",
			Temperature: 0.2,
			MaxTokens:   1024,
		},
	})
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	event, err := client.ExtractEvent(context.Background(), strings.NewReader("fake-png-data"), MIMEType("image/png"))
	if err != nil {
		t.Fatalf("ExtractEvent() error: %v", err)
	}
	if event.Title != "Image Event" {
		t.Errorf("Title = %q, want %q", event.Title, "Image Event")
	}
}

func TestNewClient_InvalidProvider(t *testing.T) {
	_, err := NewClient(config.LLMConfig{Provider: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
}

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"title":"hi"}`, `{"title":"hi"}`},
		{"```json\n{\"title\":\"hi\"}\n```", `{"title":"hi"}`},
		{"```\n{\"title\":\"hi\"}\n```", `{"title":"hi"}`},
	}
	for _, tc := range tests {
		got := stripCodeFence(tc.input)
		if got != tc.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
