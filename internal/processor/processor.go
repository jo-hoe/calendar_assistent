package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/jo-hoe/calendar-assistent/internal/calendar"
	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

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

func (p *Processor) ProcessArtifact(ctx context.Context, r io.Reader, mimeType string) (*Result, error) {
	event, err := p.llmClient.ExtractEvent(ctx, r, mimeType)
	if err != nil {
		return nil, fmt.Errorf("extracting event: %w", err)
	}

	if event.Title == "" {
		return nil, fmt.Errorf("could not extract a valid calendar event from the provided content")
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
