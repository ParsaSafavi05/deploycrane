package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	model "github.com/ParsaSafavi05/deploycrane/internal/models"
)

var ErrNotFound = errors.New("app not found")

type InMemoryStore struct {
	mu  sync.RWMutex
	app map[string]model.App
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		app: make(map[string]model.App),
	}
}

func (s *InMemoryStore) Create(ctx context.Context, app model.App) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.app[app.ID] = app
	return nil
}

func (s *InMemoryStore) Get(ctx context.Context, id string) (model.App, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return model.App{}, ctx.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	app, exists := s.app[id]
	if !exists {
		return model.App{}, ErrNotFound
	}

	return app, nil
}

func (s *InMemoryStore) Update(ctx context.Context, id string, fn func(*model.App)) error {
    if ctx.Err() != nil {
        return ctx.Err()
    }
    s.mu.Lock()
    defer s.mu.Unlock()
	
    app, ok := s.app[id]
    if !ok {
        return ErrNotFound
    }
    fn(&app)
    s.app[id] = app
    return nil
}

func (s *InMemoryStore) List(ctx context.Context) ([]model.App, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	apps := make([]model.App, 0, len(s.app))
	for _, app := range s.app {
		apps = append(apps, app)
	}

	return apps, nil
}

func (s *InMemoryStore) Delete(ctx context.Context, id string) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.app[id]; !exists {
		return ErrNotFound
	}

	delete(s.app, id)
	return nil
}

func (s *InMemoryStore) Count(ctx context.Context) (int, error) {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.app), nil
}

func (s *InMemoryStore) Ping(ctx context.Context) error {
	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Acquire read lock
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Ensure map is initialised
	if s.app == nil {
		return fmt.Errorf("store not initialised")
	}

	// The store is available
	return nil
}
