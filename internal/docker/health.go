package docker

import (
	"context"
	"fmt"

	"github.com/ParsaSafavi05/deploycrane/internal/health"
	"github.com/moby/moby/client"
)

type dockerChecker struct {
	cli client.APIClient
}

func NewHealthChecker(c client.APIClient) health.Checker {
	return &dockerChecker{
		cli: c,
	}
}

func (d *dockerChecker) Name() string {
	return "docker"
}

func (d *dockerChecker) Check(ctx context.Context) error {
	_, err := d.cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		return fmt.Errorf("docker daemon unreachable: %w", err)
	}
	return nil
}
