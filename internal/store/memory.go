package store

import (
	"context"
	"fmt"
	"sync"

	"github.com/ParsaSafavi05/deploycrane/internal/model"
)

type InMemoryStore struct {
	mu sync.RWMutex
	app map[string]model.App
}

func NewInMemoryStore() *InMemoryStore  {
	return &InMemoryStore{
		app: make(map[string]model.App),
	}
}

func (s *InMemoryStore) Create(ctx context.Context, app model.App) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app[app.ID] = app
	return nil
}

func (s *InMemoryStore) Get(ctx context.Context, id string) (model.App, error) {
	return model.App{}, fmt.Errorf("not implemented")
}

func (s *InMemoryStore) Update(ctx context.Context, app model.App) error{
	return fmt.Errorf("not implemented")
}

func (s *InMemoryStore) List(ctx context.Context) ([]model.App, error)  {
	return nil, fmt.Errorf("not implemented")
}

func (s *InMemoryStore) Delete(ctx context.Context, id string) error{
	return fmt.Errorf("not implemented")
}

func (s *InMemoryStore) Count(ctx context.Context) (int, error){
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.app), nil
}
