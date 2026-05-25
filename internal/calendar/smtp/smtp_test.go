package smtp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockDialer captures calls to send for assertions.
type mockDialer struct {
	sentBody    []byte
	sentSubject string
	sentFrom    string
	sentTo      string
	returnErr   error
}

func (m *mockDialer) send(_ context.Context, _ string, _ int, from, to, subject string, body []byte) error {
	if m.returnErr != nil {
		return m.returnErr
	}
	m.sentFrom = from
	m.sentTo = to
	m.sentSubject = subject
	m.sentBody = bytes.Clone(body)
	return nil
}

func newTestProvider(creds *SMTPCredentials, d dialer) *smtpProvider {
	cfg := config.SMTPConfig{
		Host:       "smtp.example.com",
		Port:       587,
		AuthMethod: config.SMTPAuthPlain,
		From:       "sender@example.com",
		To:         "recipient@example.com",
	}
	return &smtpProvider{
		cfg:    cfg,
		creds:  creds,
		dialer: d,
		logger: newDiscardLogger(),
	}
}

func newTestEvent() *llm.EventData {
	return &llm.EventData{
		Title:       "Project Kickoff",
		Description: "Initial project meeting",
		StartTime:   time.Date(2026, 7, 1, 14, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 7, 1, 15, 0, 0, 0, time.UTC),
	}
}

func TestCreateEvent_NoError(t *testing.T) {
	md := &mockDialer{}
	p := newTestProvider(nil, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateEvent_SentBodyContainsMethodRequest(t *testing.T) {
	md := &mockDialer{}
	p := newTestProvider(nil, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(md.sentBody), "METHOD:REQUEST") {
		t.Errorf("expected METHOD:REQUEST in sent body, got:\n%s", md.sentBody)
	}
}

func TestCreateEvent_SubjectPrefixed(t *testing.T) {
	md := &mockDialer{}
	p := newTestProvider(nil, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Invitation: Project Kickoff"
	if md.sentSubject != want {
		t.Errorf("subject: got %q, want %q", md.sentSubject, want)
	}
}

func TestCreateEvent_OrganizerFallsBackToCfgFrom(t *testing.T) {
	// creds with empty organizer — should fall back to cfg.From.
	creds := &SMTPCredentials{Username: "u", Password: "p", Organizer: ""}
	md := &mockDialer{}
	p := newTestProvider(creds, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := string(md.sentBody)
	if !strings.Contains(body, "ORGANIZER:mailto:sender@example.com") {
		t.Errorf("expected ORGANIZER to fall back to cfg.From, got body:\n%s", body)
	}
}

func TestCreateEvent_OrganizerFromCreds(t *testing.T) {
	creds := &SMTPCredentials{Username: "u", Password: "p", Organizer: "mailto:organizer@example.com"}
	md := &mockDialer{}
	p := newTestProvider(creds, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := string(md.sentBody)
	if !strings.Contains(body, "ORGANIZER:mailto:organizer@example.com") {
		t.Errorf("expected ORGANIZER from creds, got body:\n%s", body)
	}
}

func TestCreateEvent_DialerError_Propagated(t *testing.T) {
	md := &mockDialer{returnErr: context.DeadlineExceeded}
	p := newTestProvider(nil, md)

	_, err := p.CreateEvent(context.Background(), newTestEvent())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sending calendar invitation") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestCreateEvent_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Test via netDialer which checks ctx.Err() before dialing.
	nd := &netDialer{
		cfg:   config.SMTPConfig{Host: "localhost", Port: 9999},
		creds: nil,
	}
	err := nd.send(ctx, "localhost", 9999, "f@e.com", "t@e.com", "subj", []byte("body"))
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled before dial") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveOrganizer_NilCreds(t *testing.T) {
	got := resolveOrganizer(nil, "fallback@example.com")
	if got != "fallback@example.com" {
		t.Errorf("got %q, want %q", got, "fallback@example.com")
	}
}

func TestResolveOrganizer_EmptyOrganizerInCreds(t *testing.T) {
	creds := &SMTPCredentials{Organizer: ""}
	got := resolveOrganizer(creds, "fallback@example.com")
	if got != "fallback@example.com" {
		t.Errorf("got %q, want %q", got, "fallback@example.com")
	}
}

func TestResolveOrganizer_CredsOrganizer(t *testing.T) {
	creds := &SMTPCredentials{Organizer: "org@example.com"}
	got := resolveOrganizer(creds, "fallback@example.com")
	if got != "org@example.com" {
		t.Errorf("got %q, want %q", got, "org@example.com")
	}
}

func TestLoginAuth_Start(t *testing.T) {
	auth := newLoginAuth("testuser", "testpass")
	mech, resp, err := auth.Start(nil)
	if mech != "LOGIN" {
		t.Errorf("Start() mechanism = %q, want %q", mech, "LOGIN")
	}
	if len(resp) != 0 {
		t.Errorf("Start() resp = %v, want empty bytes", resp)
	}
	if err != nil {
		t.Errorf("Start() err = %v, want nil", err)
	}
}

func TestLoginAuth_Next_Username(t *testing.T) {
	auth := newLoginAuth("testuser", "testpass")
	resp, err := auth.Next([]byte("Username:"), true)
	if err != nil {
		t.Fatalf("Next(Username:, true) error: %v", err)
	}
	if string(resp) != "testuser" {
		t.Errorf("Next(Username:, true) = %q, want %q", string(resp), "testuser")
	}
}

func TestLoginAuth_Next_Password(t *testing.T) {
	auth := newLoginAuth("testuser", "testpass")
	resp, err := auth.Next([]byte("Password:"), true)
	if err != nil {
		t.Fatalf("Next(Password:, true) error: %v", err)
	}
	if string(resp) != "testpass" {
		t.Errorf("Next(Password:, true) = %q, want %q", string(resp), "testpass")
	}
}

func TestLoginAuth_Next_MoreFalse(t *testing.T) {
	auth := newLoginAuth("testuser", "testpass")
	resp, err := auth.Next([]byte("anything"), false)
	if err != nil {
		t.Errorf("Next(_, false) err = %v, want nil", err)
	}
	if resp != nil {
		t.Errorf("Next(_, false) resp = %v, want nil", resp)
	}
}

func TestLoginAuth_Next_UnknownChallenge(t *testing.T) {
	auth := newLoginAuth("testuser", "testpass")
	_, err := auth.Next([]byte("Token:"), true)
	if err == nil {
		t.Error("Next(unknown challenge, true) expected error, got nil")
	}
}
