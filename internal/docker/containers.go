package docker

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
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

func StartContainer(ctx context.Context, cli client.APIClient, imageName string, portMappings map[int]int) (string, error) {
	_, err := cli.ImageInspect(ctx, imageName)
	if err != nil {
		// Image doesnt exist locally
		pullResp, err := cli.ImagePull(ctx, imageName, client.ImagePullOptions{})
		if err != nil {
			return "", err
		}

		io.Copy(io.Discard, pullResp)
		pullResp.Close()
	}

	// Setting container ports
	exposedPorts := network.PortSet{}
	portBindings := network.PortMap{}

	for containerPort, hostPort := range portMappings {
		portObj, err := network.ParsePort(strconv.Itoa(containerPort) + "/tcp")
		if err != nil {
			return "", err
		}
		exposedPorts[portObj] = struct{}{}
		portBindings[portObj] = []network.PortBinding{
			{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: strconv.Itoa(hostPort)},
		}
	}
	// Create the container
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        imageName,
			ExposedPorts: exposedPorts,
		},
		HostConfig: &container.HostConfig{
			PortBindings: portBindings,
		},
	})
	if err != nil {
		return "", err
	}

	if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return "", err
	}

	return resp.ID, nil
}

func InspectContainer(ctx context.Context, cli client.APIClient, id string) (client.ContainerInspectResult, error) {
	return cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
}

func StopContainer(ctx context.Context, cli client.APIClient, id string) (client.ContainerStopResult, error) {
	return cli.ContainerStop(ctx, id, client.ContainerStopOptions{})
}
