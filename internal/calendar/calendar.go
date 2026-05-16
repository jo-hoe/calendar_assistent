package calendar

import (
	"context"
	"fmt"

	"github.com/jo-hoe/calendar-assistent/internal/calendar/google"
	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type Provider interface {
	CreateEvent(ctx context.Context, event *llm.EventData) (string, error)
}

func NewProvider(cfg config.CalendarConfig) (Provider, error) {
	switch cfg.Provider {
	case "google":
		return google.New(cfg.Google)
	default:
		return nil, fmt.Errorf("unsupported calendar provider %q", cfg.Provider)
	}
}
