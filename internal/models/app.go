package model

import (
	"strings"
	"time"
)

type Status string

const (
	StatusCreated  Status = "created"
	StatusCloning  Status = "cloning"
	StatusCloned   Status = "cloned"
	StatusBuilding Status = "building"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusFailed   Status = "failed"
	StatusBuilt    Status = "built"
	StatusStarting Status = "starting"
)

type App struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	RepoURL       string    `json:"repo_url"`
	ClonePath     string    `json:"clone_path,omitempty"`
	Status        Status    `json:"status"`
	ContainerID   string    `json:"container_id,omitempty"`
	ContainerPort int       `json:"container_port"`
	HostPort      int       `json:"host_port"`
	CreatedAt     time.Time `json:"created_at"`
}

// internal/models/app.go
func (a *App) ApplyUpdate(name, repoURL *string, cPort, hPort *int) {
	if name != nil && *name != "" {
		a.Name = *name
	}
	if repoURL != nil {
		if strings.HasPrefix(*repoURL, "http://") || strings.HasPrefix(*repoURL, "https://") {
			a.RepoURL = *repoURL
		}
	}
	if cPort != nil && *cPort > 0 && *cPort < 65536 {
		a.ContainerPort = *cPort
	}
	if hPort != nil && *hPort > 0 && *hPort < 65536 {
		a.HostPort = *hPort
	}
}

func (a App) WouldChange(name, repoURL *string, cPort, hPort *int) bool {
	if name != nil {
		v := strings.TrimSpace(*name)
		if v != "" && v != a.Name {
			return true
		}
	}

	if repoURL != nil {
		v := strings.TrimSpace(*repoURL)
		if v != "" && v != a.RepoURL {
			return true
		}
	}

	if cPort != nil && *cPort > 0 && *cPort < 65536 && *cPort != a.ContainerPort {
		return true
	}

	if hPort != nil && *hPort > 0 && *hPort < 65536 && *hPort != a.HostPort {
		return true
	}

	return false
}