package calendar

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jo-hoe/calendar-assistent/internal/calendar/google"
	"github.com/jo-hoe/calendar-assistent/internal/calendar/webcal"
	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type Provider interface {
	CreateEvent(ctx context.Context, event *llm.EventData) (string, error)
}

func NewProvider(cfg config.CalendarConfig, logger *slog.Logger) (Provider, error) {
	switch cfg.Provider {
	case "google":
		return google.New(cfg.Google)
	case "webcal":
		return webcal.New(cfg.Webcal, logger)
	case "mock":
		return newMockProvider()
	default:
		return nil, fmt.Errorf("unsupported calendar provider %q", cfg.Provider)
	}
}
