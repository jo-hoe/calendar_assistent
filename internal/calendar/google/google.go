package google

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

const (
	tokenURL       = "https://oauth2.googleapis.com/token" //nolint:gosec // not a credential
	calendarAPIURL = "https://www.googleapis.com/calendar/v3"
	scope          = "https://www.googleapis.com/auth/calendar.events"
)

type Client struct {
	calendarID string
	timeZone   string
	creds      serviceAccountCredentials
	httpClient *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

type serviceAccountCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func New(cfg config.GoogleCalendarConfig) (*Client, error) {
	data, err := os.ReadFile(cfg.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file %q: %w", cfg.CredentialsFile, err)
	}

	var creds serviceAccountCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	if creds.ClientEmail == "" || creds.PrivateKey == "" {
		return nil, fmt.Errorf("credentials file missing client_email or private_key")
	}

	if creds.TokenURI == "" {
		creds.TokenURI = tokenURL
	}

	return &Client{
		calendarID: cfg.CalendarID,
		timeZone:   cfg.TimeZone,
		creds:      creds,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) CreateEvent(ctx context.Context, event *llm.EventData) (string, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("getting access token: %w", err)
	}

	tz := event.TimeZone
	if tz == "" {
		tz = c.timeZone
	}

	calEvent := calendarEvent{
		Summary:     event.Title,
		Description: event.Description,
		Location:    event.Location,
		Start: eventTime{
			DateTime: event.StartTime.Format(time.RFC3339),
			TimeZone: tz,
		},
		End: eventTime{
			DateTime: event.EndTime.Format(time.RFC3339),
			TimeZone: tz,
		},
	}

	body, err := json.Marshal(calEvent)
	if err != nil {
		return "", fmt.Errorf("marshaling event: %w", err)
	}

	reqURL := fmt.Sprintf("%s/calendars/%s/events", calendarAPIURL, url.PathEscape(c.calendarID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("calling calendar API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		truncated := string(respBody)
		if len(truncated) > 400 {
			truncated = truncated[:400]
		}
		return "", fmt.Errorf("calendar API returned status %d: %s", resp.StatusCode, truncated)
	}

	var result struct {
		ID      string `json:"id"`
		HTMLLink string `json:"htmlLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding calendar response: %w", err)
	}

	return result.ID, nil
}

func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	now := time.Now()
	exp := now.Add(time.Hour)

	jwt, err := c.signJWT(now, exp)
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.creds.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	c.cachedToken = tokenResp.AccessToken
	c.tokenExpiry = now.Add(time.Duration(tokenResp.ExpiresIn)*time.Second - 60*time.Second)

	return c.cachedToken, nil
}

func (c *Client) signJWT(iat, exp time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)

	claims := map[string]interface{}{
		"iss":   c.creds.ClientEmail,
		"scope": scope,
		"aud":   c.creds.TokenURI,
		"iat":   iat.Unix(),
		"exp":   exp.Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	key, err := parsePrivateKey(c.creds.PrivateKey)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

func parsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		var rsaKey *rsa.PrivateKey
		rsaKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		return rsaKey, nil
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}

type calendarEvent struct {
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       eventTime `json:"start"`
	End         eventTime `json:"end"`
}

type eventTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}
