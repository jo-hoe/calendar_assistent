package webcal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	awsV4TerminationString = "aws4_request"
	s3HTTPTimeout          = 30 * time.Second
)

type s3Credentials struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

type s3Storage struct {
	httpClient      *http.Client
	bucket          string
	key             string
	region          string
	accessKeyID     string
	secretAccessKey string
	baseURL         string
}

func newS3Storage(bucket, key, region, credentialsFile, endpoint string) (*s3Storage, error) {
	data, err := os.ReadFile(credentialsFile) //nolint:gosec // path from trusted config
	if err != nil {
		return nil, fmt.Errorf("reading S3 credentials file %q: %w", credentialsFile, err)
	}

	var creds s3Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing S3 credentials: %w", err)
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("S3 credentials file missing accessKeyId or secretAccessKey")
	}

	base := endpoint
	if base == "" {
		base = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, region)
	} else {
		base = strings.TrimRight(base, "/") + "/" + bucket
	}

	return &s3Storage{
		httpClient:      &http.Client{Timeout: s3HTTPTimeout},
		bucket:          bucket,
		key:             key,
		region:          region,
		accessKeyID:     creds.AccessKeyID,
		secretAccessKey: creds.SecretAccessKey,
		baseURL:         base,
	}, nil
}

func (s *s3Storage) Download(ctx context.Context) ([]byte, error) {
	url := s.baseURL + "/" + s.key
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating S3 GET request: %w", err)
	}

	now := time.Now().UTC()
	s.signV4(req, nil, now)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 GET: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("S3 GET returned %d: %s", resp.StatusCode, truncate(string(body), 400))
	}

	return io.ReadAll(resp.Body)
}

func (s *s3Storage) Upload(ctx context.Context, data []byte) error {
	url := s.baseURL + "/" + s.key
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating S3 PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "text/calendar")

	now := time.Now().UTC()
	s.signV4(req, data, now)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("S3 PUT: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 PUT returned %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	return nil
}

// signV4 adds AWS Signature Version 4 Authorization header to req.
func (s *s3Storage) signV4(req *http.Request, body []byte, now time.Time) {
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)

	bodyHash := hashSHA256(body)
	req.Header.Set("x-amz-content-sha256", bodyHash)

	// Canonical headers — must be sorted and lowercase.
	host := req.URL.Host
	var canonicalHeaders, signedHeaders string
	ct := req.Header.Get("Content-Type")
	if ct != "" {
		canonicalHeaders = "content-type:" + ct + "\n" +
			"host:" + host + "\n" +
			"x-amz-content-sha256:" + bodyHash + "\n" +
			"x-amz-date:" + amzDate + "\n"
		signedHeaders = "content-type;host;x-amz-content-sha256;x-amz-date"
	} else {
		canonicalHeaders = "host:" + host + "\n" +
			"x-amz-content-sha256:" + bodyHash + "\n" +
			"x-amz-date:" + amzDate + "\n"
		signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	}

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	credentialScope := dateStamp + "/" + s.region + "/s3/" + awsV4TerminationString
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + hashSHA256([]byte(canonicalRequest))

	signingKey := deriveSigningKey(s.secretAccessKey, dateStamp, s.region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKeyID, credentialScope, signedHeaders, signature,
	))
}

func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte(awsV4TerminationString))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
