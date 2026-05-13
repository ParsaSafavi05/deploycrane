package store

import (
	"context"
	"fmt"

	"github.com/ParsaSafavi05/deploycrane/internal/health"
)

type storeChecker struct {
	store Store
}

func NewHealthChecker(s Store) health.Checker {
	return &storeChecker{
		store: s,
	}
}

func (s *storeChecker) Name() string {
	return "store"
}

func (s *storeChecker) Check(ctx context.Context) error {
	err := s.store.Ping(ctx)
	if err != nil {
		return fmt.Errorf("store unreachable: %w", err)
	}
	return nil
}
