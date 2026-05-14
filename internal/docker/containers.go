package docker

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"              // New import path for the client
)

type ContainerInfo struct {
	ID     string   `json:"id"`
	Names  []string `json:"names"`
	Image  string   `json:"image"`
	State  string   `json:"state"`
	Status string   `json:"status"`
}

func ListContainers(ctx context.Context, cli client.APIClient, all bool) ([]ContainerInfo, error) {
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	containers := result.Items

	converted := make([]ContainerInfo, 0, len(containers))
    for _, c := range containers {
        converted = append(converted, ContainerInfo{
            ID:     c.ID,
            Names:  c.Names,
            Image:  c.Image,
            State:  string(c.State),
            Status: c.Status,
        })
    }
    return converted, nil
}