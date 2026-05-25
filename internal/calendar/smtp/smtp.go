package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

const (
	smtpTimeout        = 30 * time.Second
	invitationSubjPfx  = "Invitation: "
	mimeTextPlain      = "text/plain; charset=UTF-8"
	mimeTextCalendar   = "text/calendar; method=REQUEST; charset=UTF-8"
	contentDisposition = "attachment; filename=\"invite.ics\""
)

// dialer is an interface over SMTP dial+send so tests can stub it.
type dialer interface {
	send(ctx context.Context, host string, port int, from, to, subject string, body []byte) error
}

// smtpProvider implements calendar.Provider by sending a METHOD:REQUEST
// iCalendar invitation via SMTP.
type smtpProvider struct {
	cfg    config.SMTPConfig
	creds  *SMTPCredentials // nil when authMethod is "none"
	dialer dialer
	logger *slog.Logger
}

// New constructs an smtpProvider, loading credentials when required.
func New(cfg config.SMTPConfig, logger *slog.Logger) (*smtpProvider, error) {
	if logger == nil {
		logger = slog.Default()
	}
	creds, err := loadCredsIfNeeded(cfg)
	if err != nil {
		return nil, err
	}
	return &smtpProvider{
		cfg:    cfg,
		creds:  creds,
		dialer: newNetDialer(cfg, creds),
		logger: logger,
	}, nil
}

func loadCredsIfNeeded(cfg config.SMTPConfig) (*SMTPCredentials, error) {
	if cfg.AuthMethod == config.SMTPAuthNone {
		return nil, nil
	}
	creds, err := loadCredentials(cfg.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("loading SMTP credentials: %w", err)
	}
	if creds.Username == "" || creds.Password == "" {
		return nil, fmt.Errorf("SMTP credentials file missing username or password")
	}
	return creds, nil
}

// CreateEvent builds an iCalendar invitation and sends it via SMTP.
func (p *smtpProvider) CreateEvent(ctx context.Context, event *llm.EventData) (string, error) {
	organizer := resolveOrganizer(p.creds, p.cfg.From)
	ics := buildICS(event, organizer, p.cfg.To)
	subject := invitationSubjPfx + sanitizeHeader(event.Title)

	if err := p.dialer.send(ctx, p.cfg.Host, p.cfg.Port, p.cfg.From, p.cfg.To, subject, ics); err != nil {
		return "", fmt.Errorf("sending calendar invitation: %w", err)
	}
	p.logger.Info("calendar invitation sent", "to", p.cfg.To, "subject", subject)
	return "", nil
}

func resolveOrganizer(creds *SMTPCredentials, fallback string) string {
	if creds != nil && creds.Organizer != "" {
		return creds.Organizer
	}
	return fallback
}

// ---------- real net/smtp dialer ----------

type netDialer struct {
	cfg   config.SMTPConfig
	creds *SMTPCredentials // nil when authMethod is "none"
}

func newNetDialer(cfg config.SMTPConfig, creds *SMTPCredentials) *netDialer {
	return &netDialer{cfg: cfg, creds: creds}
}

func (d *netDialer) send(ctx context.Context, host string, port int, from, to, subject string, icsBody []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled before dial: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := d.dial(addr)
	if err != nil {
		return fmt.Errorf("dialing SMTP server: %w", err)
	}
	defer func() { _ = client.Quit() }()

	if err := d.authenticate(client, host); err != nil {
		return err
	}
	return sendMessage(client, from, to, subject, icsBody)
}

func (d *netDialer) dial(addr string) (*smtp.Client, error) {
	if d.cfg.TLS {
		return dialTLS(addr, d.cfg.Host)
	}
	return dialPlain(addr, d.cfg.Host)
}

func dialTLS(addr, host string) (*smtp.Client, error) {
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12} //nolint:gosec // ServerName is set
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: smtpTimeout}, "tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("TLS dial %s: %w", addr, err)
	}
	return smtp.NewClient(conn, host)
}

func dialPlain(addr, host string) (*smtp.Client, error) {
	client, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12} //nolint:gosec // ServerName is set
		if err := client.StartTLS(tlsCfg); err != nil {
			return nil, fmt.Errorf("STARTTLS: %w", err)
		}
	}
	return client, nil
}

func (d *netDialer) authenticate(client *smtp.Client, host string) error {
	if d.creds == nil {
		return nil
	}
	switch d.cfg.AuthMethod {
	case config.SMTPAuthPlain:
		return client.Auth(smtp.PlainAuth("", d.creds.Username, d.creds.Password, host))
	case config.SMTPAuthLogin:
		return client.Auth(newLoginAuth(d.creds.Username, d.creds.Password))
	default:
		return fmt.Errorf("unsupported SMTP auth method %q", d.cfg.AuthMethod)
	}
}

func sendMessage(client *smtp.Client, from, to, subject string, icsBody []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	return writeData(client, from, to, subject, icsBody)
}

func writeData(client *smtp.Client, from, to, subject string, icsBody []byte) error {
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command: %w", err)
	}
	defer func() { _ = w.Close() }()

	msg, err := buildMIMEMessage(from, to, subject, icsBody)
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing message body: %w", err)
	}
	return nil
}

// ---------- MIME message builder ----------

func buildMIMEMessage(from, to, subject string, icsBody []byte) ([]byte, error) {
	var buf bytes.Buffer
	boundary, err := writeMIMEHeaders(&buf, from, to, subject)
	if err != nil {
		return nil, err
	}
	mw := multipart.NewWriter(&buf)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("setting boundary: %w", err)
	}
	if err := writeTextPart(mw, icsBody); err != nil {
		return nil, err
	}
	if err := writeCalendarPart(mw, icsBody); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}
	return buf.Bytes(), nil
}

func writeMIMEHeaders(buf *bytes.Buffer, from, to, subject string) (string, error) {
	// Use a temporary writer just to get a boundary string.
	tmp := multipart.NewWriter(new(bytes.Buffer))
	boundary := tmp.Boundary()
	fmt.Fprintf(buf, "From: %s\r\nTo: %s\r\nSubject: %s\r\n", from, to, encodeSubject(subject))
	fmt.Fprintf(buf, "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n", boundary)
	return boundary, nil
}

func writeTextPart(mw *multipart.Writer, icsBody []byte) error {
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Type", mimeTextPlain)
	hdr.Set("Content-Transfer-Encoding", "quoted-printable")
	pw, err := mw.CreatePart(hdr)
	if err != nil {
		return fmt.Errorf("creating text part: %w", err)
	}
	qw := quotedprintable.NewWriter(pw)
	_, err = qw.Write([]byte("You have received a calendar invitation.\r\n\r\n" + string(icsBody)))
	if err != nil {
		return fmt.Errorf("writing text part body: %w", err)
	}
	if err := qw.Close(); err != nil {
		return fmt.Errorf("closing text part: %w", err)
	}
	return nil
}

func writeCalendarPart(mw *multipart.Writer, icsBody []byte) error {
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Type", mimeTextCalendar)
	hdr.Set("Content-Transfer-Encoding", "8bit")
	hdr.Set("Content-Disposition", contentDisposition)
	pw, err := mw.CreatePart(hdr)
	if err != nil {
		return fmt.Errorf("creating calendar part: %w", err)
	}
	if _, err := pw.Write(icsBody); err != nil {
		return fmt.Errorf("writing calendar part: %w", err)
	}
	return nil
}

// ---------- subject encoding ----------

// sanitizeHeader strips bare CR and LF characters to prevent CRLF injection
// in MIME headers built from user-controlled input.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// encodeSubject returns subject unchanged for ASCII or RFC 2047 UTF-8 QP.
func encodeSubject(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?Q?" + encodeQP(s) + "?="
		}
	}
	return s
}

func encodeQP(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if isQPSafe(b) {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "=%02X", b)
		}
	}
	return sb.String()
}

func isQPSafe(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// ---------- LOGIN auth ----------

// loginAuth implements the SMTP LOGIN authentication mechanism.
type loginAuth struct {
	username string
	password string
}

func newLoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username: username, password: password}
}

func (a *loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN server challenge: %q", fromServer)
	}
}
