package docker

import (
	"context"

	"github.com/moby/moby/client"
)

// NewClient creates a docker client using environment defaults and API version negotiation
func NewClient() (client.APIClient, error) {
	return client.New(client.FromEnv)
}

// Checks whether the docker daemon is reachable
func Ping(ctx context.Context, cli *client.Client) error  {
	_, err := cli.Ping(ctx, client.PingOptions{})
	return err
}