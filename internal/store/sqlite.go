package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) the SQLite database at dbPath
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// SQLite doesn't handle concurrent writes; one connection is enough
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Retry writes for up to 5s instead of failing immediately
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	// Create the apps table if it doesn't exist
	schema := `CREATE TABLE IF NOT EXISTS apps (
		id              TEXT PRIMARY KEY,
		name            TEXT    NOT NULL,
		repo_url        TEXT    NOT NULL,
		clone_path      TEXT,
		status          TEXT    NOT NULL DEFAULT 'created',
		container_id    TEXT,
		container_port  INTEGER DEFAULT 0,
		host_port       INTEGER DEFAULT 0,
		created_at      TEXT    NOT NULL
	)`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Create(ctx context.Context, app model.App) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO apps 
		(id, name, repo_url, clone_path, status, container_id, container_port, host_port, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.ID,
		app.Name,
		app.RepoURL,
		app.ClonePath,
		app.Status,
		app.ContainerID,
		app.ContainerPort,
		app.HostPort,
		app.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("sqlite create: %w", err)
	}

	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (model.App, error) {
	if ctx.Err() != nil {
		return model.App{}, ctx.Err()
	}

	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, name, repo_url, clone_path, status, container_id, container_port, host_port, created_at
		FROM apps WHERE id = ?`,
		id,
	)

	var app model.App
	var createdAt string

	err := row.Scan(
		&app.ID, &app.Name, &app.RepoURL, &app.ClonePath, &app.Status,
		&app.ContainerID, &app.ContainerPort, &app.HostPort, &createdAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return model.App{}, ErrNotFound
	}

	if err != nil {
		return model.App{}, fmt.Errorf("sqlite get: %w", err)
	}

	app.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return model.App{}, fmt.Errorf("parse created_at: %w", err)
	}
	return app, nil
}

func (s *SQLiteStore) Update(ctx context.Context, id string, fn func(*model.App)) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Start a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite update begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var app model.App
	var createdAt string

	// Retrieve the current record
	row := tx.QueryRowContext(
		ctx,
		`SELECT id, name, repo_url, clone_path, status, container_id, container_port, host_port, created_at
		 FROM apps WHERE id = ?`,
		id,
	)

	err = row.Scan(
		&app.ID, &app.Name, &app.RepoURL, &app.ClonePath, &app.Status,
		&app.ContainerID, &app.ContainerPort, &app.HostPort, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("sqlite update select: %w", err)
	}
	app.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}

	// Apply the caller's mutation
	fn(&app)

	// Write back the updated record
	_, err = tx.ExecContext(ctx,
		`UPDATE apps SET name=?, repo_url=?, clone_path=?, status=?, container_id=?, container_port=?, host_port=?, created_at=?
		 WHERE id=?`,
		app.Name, app.RepoURL, app.ClonePath, app.Status,
		app.ContainerID, app.ContainerPort, app.HostPort,
		app.CreatedAt.Format(time.RFC3339), app.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite update exec: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) List(ctx context.Context) ([]model.App, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, repo_url, clone_path, status, container_id, container_port, host_port, created_at
		 FROM apps`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite list: %w", err)
	}
	defer rows.Close()

	var apps []model.App
	for rows.Next() {
		var app model.App
		var createdAt string
		if err := rows.Scan(
			&app.ID, &app.Name, &app.RepoURL, &app.ClonePath, &app.Status,
			&app.ContainerID, &app.ContainerPort, &app.HostPort, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite list scan: %w", err)
		}
		app.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite list rows: %w", err)
	}
	if apps == nil {
		apps = []model.App{}
	}
	return apps, nil
}

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("sqlite delete: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) Count(ctx context.Context) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM apps`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite count: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}