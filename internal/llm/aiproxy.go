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

func (c *aiProxyClient) ExtractEvent(ctx context.Context, r io.Reader, mimeType string) (*EventData, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	var userParts []messagePart
	if strings.HasPrefix(mimeType, "text/") {
		userParts = []messagePart{
			{Type: partTypeText, Text: string(data)},
		}
	} else {
		encoded := base64.StdEncoding.EncodeToString(data)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
		userParts = []messagePart{
			{Type: partTypeImageURL, ImageURL: &imageURL{URL: dataURL}},
			{Type: partTypeText, Text: "Extract the calendar event from this image/document."},
		}
	}

	req := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: roleSystem, Content: []messagePart{{Type: partTypeText, Text: c.sysPrompt}}},
			{Role: roleUser, Content: userParts},
		},
		Temperature: c.temperature,
		MaxTokens:   c.maxTokens,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling AI proxy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		truncated := string(respBody)
		if len(truncated) > 400 {
			truncated = truncated[:400]
		}
		return nil, fmt.Errorf("AI proxy returned status %d: %s", resp.StatusCode, truncated)
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding AI proxy response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("AI proxy returned no choices")
	}

	content := chatResp.Choices[0].Message.Content
	content = strings.TrimSpace(content)
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

func stripCodeFence(s string) string {
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) > 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
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

const (
	roleSystem       = "system"
	roleUser         = "user"
	partTypeText     = "text"
	partTypeImageURL = "image_url"
)

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []messagePart `json:"content"`
}

type messagePart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
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
