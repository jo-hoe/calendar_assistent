package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
)

// messageRole is a typed string for chat message roles.
type messageRole string

// contentPartType is a typed string for content part types.
type contentPartType string

const (
	roleSystem       messageRole     = "system"
	roleUser         messageRole     = "user"
	partTypeText     contentPartType = "text"
	partTypeImageURL contentPartType = "image_url"
)

const (
	// maxErrorBodyLen limits how many bytes of an error response body are included in error messages.
	maxErrorBodyLen = 400
	// codeFenceMarker is the markdown code fence delimiter.
	codeFenceMarker = "```"
)

type aiProxyClient struct {
	baseURL     string
	apiKey      string
	model       string
	sysPrompt   string
	temperature float64
	maxTokens   int
	httpClient  *http.Client
}

func newAIProxyFromConfig(cfg config.AIProxyConfig) (Client, error) {
	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = DefaultPrompt
	}
	return &aiProxyClient{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		sysPrompt:   sysPrompt,
		temperature: cfg.Temperature,
		maxTokens:   cfg.MaxTokens,
		httpClient:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// prepareContent reads the reader, base64-encodes binary data, and returns the
// appropriate user content parts for the given MIME type.
func (c *aiProxyClient) prepareContent(r io.Reader, mimeType MIMEType) ([]messagePart, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	if strings.HasPrefix(string(mimeType), "text/") {
		return []messagePart{
			{Type: partTypeText, Text: string(data)},
		}, nil
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	return []messagePart{
		{Type: partTypeImageURL, ImageURL: &imageURL{URL: dataURL}},
		{Type: partTypeText, Text: "Extract the calendar event from this image/document."},
	}, nil
}

// callAPI sends the chat completion request and returns the raw content string.
func (c *aiProxyClient) callAPI(ctx context.Context, messages []chatMessage) (string, error) {
	req := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("calling AI proxy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// best-effort read for error context
		respBody, _ := io.ReadAll(resp.Body)
		truncated := string(respBody)
		if len(truncated) > maxErrorBodyLen {
			truncated = truncated[:maxErrorBodyLen]
		}
		return "", fmt.Errorf("AI proxy returned status %d: %s", resp.StatusCode, truncated)
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decoding AI proxy response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("AI proxy returned no choices")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// parseEventJSON strips code fences, unmarshals the JSON, and validates required fields.
func (c *aiProxyClient) parseEventJSON(content string) (*EventData, error) {
	content = stripCodeFence(content)

	var event EventData
	if err := json.Unmarshal([]byte(content), &event); err != nil {
		return nil, fmt.Errorf("parsing event from LLM response: %w (raw: %s)", err, truncate(content, 200))
	}

	if event.Title == "" {
		return nil, fmt.Errorf("LLM returned empty event title")
	}

	return &event, nil
}

// ExtractEvent orchestrates content preparation, API call, and JSON parsing.
func (c *aiProxyClient) ExtractEvent(ctx context.Context, r io.Reader, mimeType MIMEType) (*EventData, error) {
	userParts, err := c.prepareContent(r, mimeType)
	if err != nil {
		return nil, err
	}

	messages := []chatMessage{
		{Role: roleSystem, Content: []messagePart{{Type: partTypeText, Text: c.sysPrompt}}},
		{Role: roleUser, Content: userParts},
	}

	content, err := c.callAPI(ctx, messages)
	if err != nil {
		return nil, err
	}

	return c.parseEventJSON(content)
}

func stripCodeFence(s string) string {
	if strings.HasPrefix(s, codeFenceMarker) {
		lines := strings.Split(s, "\n")
		if len(lines) > 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), codeFenceMarker) {
			lines = lines[:len(lines)-1]
		}
		return strings.Join(lines, "\n")
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    messageRole   `json:"role"`
	Content []messagePart `json:"content"`
}

type messagePart struct {
	Type     contentPartType `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *imageURL       `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message responseMsg `json:"message"`
}

type responseMsg struct {
	Content string `json:"content"`
}
