package model

import "time"

type Status string

const (
	StatusCreated  Status = "created"
	StatusCloning  Status = "cloning"
	StatusCloned  Status = "cloned"
	StatusBuilding Status = "building"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusFailed   Status = "failed"
	StatusBuilt     Status = "built"
	StatusStarting Status = "starting"
)

type App struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	RepoURL     string    `json:"repo_url"`
	ClonePath     string    `json:"clone_path,omitempty"`
	Status      Status    `json:"status"`
	ContainerID string    `json:"container_id,omitempty"`
	Port        int       `json:"port,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
