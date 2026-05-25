package calendar

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jo-hoe/calendar-assistent/internal/calendar/google"
	smtppkg "github.com/jo-hoe/calendar-assistent/internal/calendar/smtp"
	"github.com/jo-hoe/calendar-assistent/internal/calendar/webcal"
	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type Provider interface {
	CreateEvent(ctx context.Context, event *llm.EventData) (string, error)
}

func NewProvider(cfg config.CalendarConfig, logger *slog.Logger) (Provider, error) {
	switch cfg.Provider {
	case config.CalendarProviderGoogle:
		return google.New(cfg.Google)
	case config.CalendarProviderWebcal:
		return webcal.New(cfg.Webcal, logger)
	case config.CalendarProviderSMTP:
		return smtppkg.New(cfg.SMTP, logger)
	case config.CalendarProviderMock:
		return newMockProvider()
	default:
		return nil, fmt.Errorf("unsupported calendar provider %q", cfg.Provider)
	}
}
