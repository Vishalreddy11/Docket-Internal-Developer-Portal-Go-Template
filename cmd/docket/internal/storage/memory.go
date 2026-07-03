package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
)

type memEntry struct {
	data        []byte
	contentType string
}

type memoryStorage struct {
	mu   sync.RWMutex
	data map[string]memEntry
}

func newMemoryStorage() *memoryStorage { return &memoryStorage{data: map[string]memEntry{}} }

func (m *memoryStorage) Put(_ context.Context, id string, r io.Reader, _ int64, ct string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[id] = memEntry{data: b, contentType: ct}
	return nil
}

func (m *memoryStorage) Get(_ context.Context, id string) (io.ReadCloser, int64, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.data[id]
	if !ok {
		return nil, 0, "", errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(e.data)), int64(len(e.data)), e.contentType, nil
}

func (m *memoryStorage) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}

func (m *memoryStorage) Mode() string                  { return "memory" }
func (m *memoryStorage) Close(_ context.Context) error { return nil }
