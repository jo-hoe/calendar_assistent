package webcal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jo-hoe/calendar-assistent/internal/config"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type storage interface {
	Download(ctx context.Context) ([]byte, error)
	Upload(ctx context.Context, data []byte) error
}

type webcalProvider struct {
	store     storage
	ttl       time.Duration
	publicURL string
	logger    *slog.Logger
	mu        sync.Mutex
}

func New(cfg config.WebcalConfig, logger *slog.Logger) (*webcalProvider, error) {
	var store storage
	var publicURL string
	switch cfg.Storage.Provider {
	case "s3":
		s3, err := newS3Storage(
			cfg.Storage.S3.Bucket,
			cfg.Storage.S3.Key,
			cfg.Storage.S3.Region,
			cfg.Storage.S3.CredentialsFile,
			cfg.Storage.S3.Endpoint,
		)
		if err != nil {
			return nil, fmt.Errorf("creating S3 storage: %w", err)
		}
		store = s3
		publicURL = cfg.Storage.S3.PublicURL
	case "mock":
		store = &MockStorage{}
	default:
		return nil, fmt.Errorf("unsupported webcal storage provider %q", cfg.Storage.Provider)
	}

	return &webcalProvider{
		store:     store,
		ttl:       cfg.EventTTL.Duration,
		publicURL: publicURL,
		logger:    logger,
	}, nil
}

func (p *webcalProvider) CreateEvent(ctx context.Context, event *llm.EventData) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := p.store.Download(ctx)
	if err != nil {
		return "", fmt.Errorf("downloading .ics: %w", err)
	}

	var events []vevent
	if len(data) > 0 {
		events, err = parseICS(data)
		if err != nil {
			return "", fmt.Errorf("parsing .ics: %w", err)
		}
	}

	events = pruneExpired(events, p.ttl)
	events = append(events, newVEvent(event))

	out := serializeICS(events)
	if err := p.store.Upload(ctx, out); err != nil {
		return "", fmt.Errorf("uploading .ics: %w", err)
	}

	args := []any{"events", len(events)}
	if p.publicURL != "" {
		args = append(args, "url", p.publicURL)
	}
	logger := p.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("webcal .ics updated", args...)

	return "", nil
}
