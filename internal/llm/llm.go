package llm

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
)

type EventData struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	Location    string    `json:"location"`
	TimeZone    string    `json:"timeZone"`
}

type Client interface {
	ExtractEvent(ctx context.Context, r io.Reader, mimeType string) (*EventData, error)
}

func NewClient(cfg config.LLMConfig) (Client, error) {
	switch cfg.Provider {
	case "mock":
		return newMockFromConfig()
	case "aiproxy":
		return newAIProxyFromConfig(cfg.AIProxy)
	default:
		return nil, fmt.Errorf("unsupported llm provider %q", cfg.Provider)
	}
}
