package processor

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jo-hoe/calendar-assistent/internal/calendar"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

// ErrCannotExtract is returned when event data cannot be extracted from the input.
var ErrCannotExtract = errors.New("could not extract event from input")

type Processor struct {
	llmClient   llm.Client
	calProvider calendar.Provider
}

func New(llmClient llm.Client, calProvider calendar.Provider) *Processor {
	return &Processor{
		llmClient:   llmClient,
		calProvider: calProvider,
	}
}

type Result struct {
	EventID   string         `json:"eventId"`
	EventData *llm.EventData `json:"eventData"`
}

func (p *Processor) ProcessArtifact(ctx context.Context, r io.Reader, mimeType llm.MIMEType) (*Result, error) {
	event, err := p.llmClient.ExtractEvent(ctx, r, mimeType)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotExtract, err)
	}

	if event.Title == "" {
		return nil, ErrCannotExtract
	}

	eventID, err := p.calProvider.CreateEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("creating calendar event: %w", err)
	}

	return &Result{
		EventID:   eventID,
		EventData: event,
	}, nil
}
