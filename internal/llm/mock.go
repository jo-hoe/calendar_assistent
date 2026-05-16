package llm

import (
	"context"
	"io"
	"time"
)

type mockClient struct{}

func newMockFromConfig() (Client, error) {
	return &mockClient{}, nil
}

func (m *mockClient) ExtractEvent(_ context.Context, _ io.Reader, _ string) (*EventData, error) {
	now := time.Now().Add(24 * time.Hour)
	return &EventData{
		Title:       "Mock Event",
		Description: "This is a mock event created for testing",
		StartTime:   time.Date(now.Year(), now.Month(), now.Day(), 14, 0, 0, 0, time.UTC),
		EndTime:     time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC),
		Location:    "Mock Location",
		TimeZone:    "UTC",
	}, nil
}
