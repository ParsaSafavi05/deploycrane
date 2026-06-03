package service

import (
	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/moby/moby/client"
)

type AppService struct {
	store        store.Store
	dockerClient client.APIClient
	watcher      *docker.ContainerWatcher
	pm           *docker.PortManager
	cfg          config.Config
}

func NewAppService(
	store store.Store,
	dockerClient client.APIClient,
	watcher *docker.ContainerWatcher,
	pm *docker.PortManager,
	cfg config.Config,
) *AppService {
	return &AppService{
		store:        store,
		dockerClient: dockerClient,
		watcher:      watcher,
		pm:           pm,
		cfg:          cfg,
	}
}