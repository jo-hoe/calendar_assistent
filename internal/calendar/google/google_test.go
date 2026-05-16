package google

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

func TestClient_CreateEvent(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	calendarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth: %s", r.Header.Get("Authorization"))
		}

		var event calendarEvent
		_ = json.NewDecoder(r.Body).Decode(&event)

		if event.Summary != "Test Event" {
			t.Errorf("Summary = %q, want %q", event.Summary, "Test Event")
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":       "event123",
			"htmlLink": "https://calendar.google.com/event/123",
		})
	}))
	defer calendarServer.Close()

	credsFile := writeTempCredentials(t, tokenServer.URL, keyPEM)

	client, err := New(config.GoogleCalendarConfig{
		CredentialsFile: credsFile,
		CalendarID:      "primary",
		TimeZone:        "UTC",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	client.creds.TokenURI = tokenServer.URL

	eventData := &llm.EventData{
		Title:       "Test Event",
		Description: "A test",
		StartTime:   time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Location:    "Room 1",
		TimeZone:    "UTC",
	}

	client.httpClient = &http.Client{
		Transport: &testTransport{
			tokenURL:    tokenServer.URL,
			calendarURL: calendarServer.URL,
		},
		Timeout: 10 * time.Second,
	}

	id, err := client.CreateEvent(context.Background(), eventData)
	if err != nil {
		t.Fatalf("CreateEvent() error: %v", err)
	}

	if id != "event123" {
		t.Errorf("event ID = %q, want %q", id, "event123")
	}
}

type testTransport struct {
	tokenURL    string
	calendarURL string
}

func (tr *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Host {
	case "oauth2.googleapis.com":
		newURL := tr.tokenURL + req.URL.Path
		req.URL, _ = req.URL.Parse(newURL)
	case "www.googleapis.com":
		newURL := tr.calendarURL + req.URL.Path
		req.URL, _ = req.URL.Parse(newURL)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestNew_MissingFile(t *testing.T) {
	_, err := New(config.GoogleCalendarConfig{
		CredentialsFile: "/nonexistent/path.json",
		CalendarID:      "primary",
		TimeZone:        "UTC",
	})
	if err == nil {
		t.Fatal("New() expected error for missing credentials file")
	}
}

func TestNew_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	_ = os.WriteFile(path, []byte("not json"), 0600)

	_, err := New(config.GoogleCalendarConfig{
		CredentialsFile: path,
		CalendarID:      "primary",
		TimeZone:        "UTC",
	})
	if err == nil {
		t.Fatal("New() expected error for invalid JSON")
	}
}

func TestNew_MissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	_ = os.WriteFile(path, []byte(`{"client_email":""}`), 0600)

	_, err := New(config.GoogleCalendarConfig{
		CredentialsFile: path,
		CalendarID:      "primary",
		TimeZone:        "UTC",
	})
	if err == nil {
		t.Fatal("New() expected error for missing fields")
	}
}

func writeTempCredentials(t *testing.T, tokenURI string, keyPEM []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")
	creds := map[string]string{
		"client_email": "test@test.iam.gserviceaccount.com",
		"private_key":  string(keyPEM),
		"token_uri":    tokenURI,
	}
	data, _ := json.Marshal(creds)
	_ = os.WriteFile(path, data, 0600)
	return path
}
