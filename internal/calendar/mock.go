package calendar

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jo-hoe/calendar-assistent/internal/llm"
)

type mockProvider struct {
	counter atomic.Int64
}

func newMockProvider() (Provider, error) {
	return &mockProvider{}, nil
}

func (m *mockProvider) CreateEvent(_ context.Context, _ *llm.EventData) (string, error) {
	id := m.counter.Add(1)
	return fmt.Sprintf("mock-event-%d", id), nil
}
