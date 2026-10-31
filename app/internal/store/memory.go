package store

import (
	"context"
	"errors"
	"sync"

	"github.com/jtravisp/privatepaste/internal/model"
)

// Memory is an in-memory Store for tests. It mirrors DynamoStore's semantics,
// including ConsumePaste being atomic.
type Memory struct {
	mu      sync.Mutex
	items   map[string]model.Paste
	PingErr error
}

func NewMemory() *Memory { return &Memory{items: map[string]model.Paste{}} }

func (m *Memory) CreatePaste(_ context.Context, p *model.Paste) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.items[p.ID]; exists {
		return errors.New("conditional check failed")
	}
	m.items[p.ID] = *p
	return nil
}

func (m *Memory) GetPaste(_ context.Context, id string) (*model.Paste, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (m *Memory) ConsumePaste(_ context.Context, id string) (*model.Paste, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	delete(m.items, id)
	return &p, nil
}

func (m *Memory) DeletePaste(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
	return nil
}

func (m *Memory) Ping(context.Context) error { return m.PingErr }

// Put stores a paste directly, bypassing the handler; for test setup.
func (m *Memory) Put(p model.Paste) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[p.ID] = p
}
