package store

import (
	"context"

	"github.com/ParsaSafavi05/deploycrane/internal/model"
)

type Store interface{
	Create(ctx context.Context, app model.App) error
	Get(ctx context.Context, id string) (model.App, error)
	Update(ctx context.Context, app model.App) error
	List(ctx context.Context, ) ([]model.App, error)
	Delete(ctx context.Context, id string) error 
	Count(ctx context.Context) (int, error)
}

