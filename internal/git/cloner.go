package git

import (
	"context"
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v6"
)

func Clone(ctx context.Context, url, path string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	_, err := gogit.PlainClone(path, &gogit.CloneOptions{
		URL: url,
	})
	if err != nil {
		return fmt.Errorf("clone repository: %w", err)
	}

	return nil
}
