package metadata

import (
	"context"
	"sort"
	"sync"
)

type memoryStore struct {
	mu   sync.RWMutex
	data map[string]Meta
}

func newMemoryStore() *memoryStore { return &memoryStore{data: map[string]Meta{}} }

func (m *memoryStore) Insert(_ context.Context, meta Meta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[meta.ID] = meta
	return nil
}

func (m *memoryStore) Get(_ context.Context, id string) (Meta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	meta, ok := m.data[id]
	if !ok {
		return Meta{}, ErrNotFound
	}
	return meta, nil
}

func (m *memoryStore) List(_ context.Context, limit, offset int) ([]Meta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Meta, 0, len(m.data))
	for _, v := range m.data {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UploadedAt.After(out[j].UploadedAt) })
	if offset >= len(out) {
		return []Meta{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (m *memoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}

func (m *memoryStore) Mode() string                  { return "memory" }
func (m *memoryStore) Close(_ context.Context) error { return nil }
