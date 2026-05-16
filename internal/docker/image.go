package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/moby/moby/client"
)

// ImageBuild builds a Docker image from a directory containing a Dockerfile.
func ImageBuild(ctx context.Context, cli client.APIClient, contextDir string, imageName string) (io.ReadCloser, error) {
	buildCtx, err := tarDirectory(contextDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create build context: %w", err)
	}

	options := client.ImageBuildOptions{
		Tags:           []string{imageName},
		Dockerfile:     "Dockerfile",
		Remove:         true,
		NoCache:        false,
		SuppressOutput: false,
	}

	resp, err := cli.ImageBuild(ctx, buildCtx, options)
	if err != nil {
		return nil, fmt.Errorf("image build failed: %w", err)
	}

	return resp.Body, nil
}

// tarDirectory creates a tar.gz archive of the given directory.
func tarDirectory(dir string) (io.Reader, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		header.Name = filepath.ToSlash(relPath)

		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return fmt.Errorf("failed to write file %s to tar: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		tw.Close()
		gzw.Close()
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return &buf, nil
}