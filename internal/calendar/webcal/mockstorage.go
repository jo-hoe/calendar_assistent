package webcal

import (
	"context"
	"sync"
)

// MockStorage is an in-memory storage backend. It is intended for testing and
// local development — no S3 credentials or network access required.
// Set provider: "mock" under calendar.webcal.storage in config.
type MockStorage struct {
	mu   sync.Mutex
	data []byte
}

func (m *MockStorage) Download(_ context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneBytes(m.data), nil
}

func (m *MockStorage) Upload(_ context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = cloneBytes(data)
	return nil
}

// cloneBytes returns a copy of b, safe for independent mutation.
func cloneBytes(b []byte) []byte {
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}
