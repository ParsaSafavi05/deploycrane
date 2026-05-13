package docker

import (
	"github.com/moby/moby/client"
)

// NewClient creates a docker client using environment defaults and API version negotiation
func NewClient() (client.APIClient, error) {
	return client.New(client.FromEnv)
}
