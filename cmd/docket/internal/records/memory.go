package records

import (
	"context"
	"sort"
	"sync"
)

type memoryStore struct {
	mu   sync.RWMutex
	data []Record
}

func newMemoryStore() *memoryStore { return &memoryStore{} }

func (m *memoryStore) Insert(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append(m.data, r)
	return nil
}

func (m *memoryStore) ListByFile(_ context.Context, fileID string) ([]Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Record
	for _, r := range m.data {
		if r.FileID == fileID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (m *memoryStore) Recent(_ context.Context, limit int) ([]Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Record, len(m.data))
	copy(out, m.data)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryStore) Mode() string                  { return "memory" }
func (m *memoryStore) Close(_ context.Context) error { return nil }
