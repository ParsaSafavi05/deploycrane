package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/git"
	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/google/uuid"
)

type input struct {
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	Deploy        string `json:"deploy"`
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
}

// Handler for listing all apps
func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	// list all apps
	apps, err := s.store.List(r.Context())

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list apps")
		return
	}

	if apps == nil {
		apps = []model.App{}
	}

	respondJSON(w, http.StatusOK, apps)

}

// Handler for getting specific app
func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	// Get app id from request
	id := r.PathValue("id")

	// Get app from store by id
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get app")
		return
	}

	respondJSON(w, http.StatusOK, app)

}

// Handler for creating the app
func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	// Decode request body
	var in input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate required fields
	in.Name = strings.TrimSpace(in.Name)
	in.RepoURL = strings.TrimSpace(in.RepoURL)
	in.Deploy = strings.TrimSpace(in.Deploy)

	if in.Name == "" || in.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "name and repo_url are required")
		return
	}

	// Basic URL validation
	if !strings.HasPrefix(in.RepoURL, "http://") && !strings.HasPrefix(in.RepoURL, "https://") {
		writeError(w, http.StatusBadRequest, "repo_url must be a valid HTTP(S) URL")
		return
	}

	// Build app model

	app := model.App{
		ID:            uuid.New().String(),
		Name:          in.Name,
		RepoURL:       in.RepoURL,
		Status:        model.StatusCreated,
		CreatedAt:     time.Now(),
		ContainerPort: in.ContainerPort,
		HostPort:      in.HostPort,
	}

	// Store the app

	if err := s.store.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save app")
		return
	}

	// Otherwise, trigger the full deploy pipeline.
	if strings.EqualFold(in.Deploy, "no") {
		respondJSON(w, http.StatusCreated, app)
		return
	}

	// Auto-deploy: the function streams progress as Server-Sent Events.
	logStreamJSON(w, http.StatusCreated)

	// Call the core deploy logic
	s.deployApp(w, r, app)

}

// Handler for cloning the app
func (s *Server) handleCloneApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch app")
		return
	}

	// Run the shared clone logic
	updatedApp, err := s.cloneApp(r.Context(), app)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("app %s cloned successfully - id: %s", updatedApp.Name, updatedApp.ID)
	respondJSON(w, http.StatusOK, updatedApp)
}

// Handler for building the app
func (s *Server) handleBuildApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// The standalone build endpoint streams build logs directly via SSE
	logStreamJSON(w, http.StatusOK)

	// Call shared build logic
	updatedApp, err := s.buildApp(r.Context(), app, w)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		return
	}

	log.Printf("app %s built successfully - tag: %s", updatedApp.Name, s.cfg.ImagePrefix+"-"+updatedApp.Name+":latest")
}

// Handler for starting the app
func (s *Server) handleStartApp(w http.ResponseWriter, r *http.Request) {

	id := r.PathValue("id")
	// Fetch current app state
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Only allow starting from certain states
	if app.Status == model.StatusStarting {
		writeError(w, http.StatusBadRequest, "app already starting")
		return
	}
	if app.Status == model.StatusRunning {
		writeError(w, http.StatusBadRequest, "app already running")
		return
	}
	if app.Status != model.StatusBuilt && app.Status != model.StatusFailed && app.Status != model.StatusStopped{
		writeError(w, http.StatusBadRequest, "app is not ready to start")
		return
	}

	containerPort := app.ContainerPort
	if containerPort <= 0 {
		// Fall back to default container port
		containerPort = s.cfg.ContainerPort
	}

	updatedApp, err := s.startApp(r.Context(), app, containerPort, app.HostPort)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("app %s started successfuly - container id: %v", updatedApp.Name, updatedApp.ContainerID)
	respondJSON(w, http.StatusOK, updatedApp)
}

// Handler for deploying the app
func (s *Server) handleDeployApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// Set up SSE
	logStreamJSON(w, http.StatusOK)
	s.deployApp(w, r, app)
}

// deployApp runs the full deployment pipeline and streams progress
func (s *Server) deployApp(w io.Writer, r *http.Request, app model.App) (model.App, error) {
	id := app.ID

	// Validate state
	switch app.Status {
	case model.StatusCreated, model.StatusFailed,
		model.StatusCloned, model.StatusBuilt, model.StatusStopped:
	case model.StatusRunning:
		return app, fmt.Errorf("app is already running")
	default:
		return app, fmt.Errorf("app is in a transitional state")
	}

	// ---- Step 1: Clone ----
	if app.Status == model.StatusCreated || app.Status == model.StatusFailed {
		updatedApp, err := s.cloneApp(r.Context(), app)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: clone failed: %s\n\n", err.Error())
			return app, err
		}
		app = updatedApp
		fmt.Fprintf(w, "data: Repository cloned successfully\n\n")
	}
	// Re-fetch the app (already done inside cloneApp, but we assign to `app`)
	app, _ = s.store.Get(r.Context(), id)

	// ---- Step 2: Build ----
	if app.Status != model.StatusBuilt && app.Status != model.StatusStopped {
		updatedApp, err := s.buildApp(r.Context(), app, w)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: build failed: %s\n\n", err.Error())
			return app, err
		}
		app = updatedApp
	}
	// No need to re-fetch; buildApp returns the updated app.

	// ---- Step 3: Start ----
	if app.Status != model.StatusRunning {
		containerPort := app.ContainerPort
		if containerPort <= 0 {
			containerPort = s.cfg.ContainerPort
		}
		updatedApp, err := s.startApp(r.Context(), app, containerPort, app.HostPort)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: start failed: %s\n\n", err.Error())
			return app, err
		}
		app = updatedApp
	}

	fmt.Fprintf(w, "event: complete\ndata: deploy finished — app is running\n\n")

	app, _ = s.store.Get(r.Context(), id)
	return app, nil
}

func (s *Server) startApp(ctx context.Context, app model.App, containerPort int, userHostPort int) (model.App, error) {
	// Validate container port
	if containerPort <= 0 || containerPort > 65535 {
		return app, fmt.Errorf("invalid container port %d", containerPort)
	}
	// Determine host port: either user-specified or automatically allocated

	var hostPort int
	var portChanged bool

	userProvided := userHostPort > 0 && userHostPort <= 65535

	if userProvided {
		// User requested a specific port
		hostPort = userHostPort
		if !s.pm.IsAvailable(hostPort) {
			var err error
			hostPort, err = s.pm.Allocate()
			if err != nil {
				return app, fmt.Errorf("requested host port %d is unavailable and no free ports", userHostPort)
			}
			portChanged = true
		}

		// Reserve the port in manager
		if err := s.pm.AllocateSpecific(hostPort); err != nil {
			return app, fmt.Errorf("failed to reserve port %d: %w", hostPort, err)
		}
	} else {
		allocated, err := s.pm.Allocate()
		if err != nil {
			// Revert status so user can retry
			s.store.Update(ctx, app.ID, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			return app, fmt.Errorf("no free ports")
		}
		hostPort = allocated
	}

	// Set status to starting
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusStarting
	}); err != nil {
		return app, fmt.Errorf("failed to start app %w", err)
	}

	imageName := s.cfg.ImagePrefix + "-" + app.Name + ":latest"
	// Start container
	portMappings := map[int]int{containerPort: hostPort}
	containerID, err := docker.StartContainer(ctx, s.dockerClient, imageName, portMappings)

	if err != nil {
		s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		return app, fmt.Errorf("failed to start container: %w", err)
	}
	s.watcher.RegisterApp(app.ID, containerID)

	// Start succeeded - save container id and set status to running
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusRunning
		a.ContainerID = containerID
		if portChanged {
			a.HostPort = hostPort
		}
	}); err != nil {
		log.Printf("CRITICAL: container started but DB update failed for app %s: %v", app.ID, err)
		return app, fmt.Errorf("container started but failed to update status: %w", err)
	}

	updatedApp, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("failed to fetch updated app")
	}

	return updatedApp, nil
}

// cloneApp clones the repository for the given app and returns the updated app.
func (s *Server) cloneApp(ctx context.Context, app model.App) (model.App, error) {
	// Validate state
	if app.Status != model.StatusCreated && app.Status != model.StatusFailed {
		return app, fmt.Errorf("app is not in a cloneable state (current: %s)", app.Status)
	}

	// Set status to cloning
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusCloning
	}); err != nil {
		return app, fmt.Errorf("failed to set status to cloning: %w", err)
	}

	// Perform the clone
	clonePath := fmt.Sprintf("%s/app-%s", s.cfg.CloneBasePath, app.ID)
	if err := git.Clone(ctx, app.RepoURL, clonePath); err != nil {
		// Mark as failed on error
		_ = s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		return app, fmt.Errorf("clone failed: %w", err)
	}

	// Success – update clone path and status
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusCloned
		a.ClonePath = clonePath
	}); err != nil {
		return app, fmt.Errorf("clone succeeded but failed to update app: %w", err)
	}

	// Return the refreshed app
	updated, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("clone succeeded but failed to fetch updated app: %w", err)
	}
	return updated, nil
}

// buildApp builds the Docker image for the given app
func (s *Server) buildApp(ctx context.Context, app model.App, w io.Writer) (model.App, error) {
	// Validate state
	if app.Status == model.StatusBuilt {
		return app, fmt.Errorf("app already built")
	}
	if app.Status != model.StatusCloned {
		return app, fmt.Errorf("app is not ready for build (current: %s)", app.Status)
	}

	// Set status to building
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusBuilding
	}); err != nil {
		return app, fmt.Errorf("failed to set status to building: %w", err)
	}

	imageName := s.cfg.ImagePrefix + "-" + app.Name + ":latest"

	// Start the build
	body, err := docker.ImageBuild(ctx, s.dockerClient, app.ClonePath, imageName)
	if err != nil {
		_ = s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		return app, fmt.Errorf("build failed: %w", err)
	}
	defer body.Close()

	// Stream build logs if a writer is provided
	if w != nil {
		if err := docker.StreamBuildLogs(w, body); err != nil {
			_ = s.store.Update(ctx, app.ID, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			return app, fmt.Errorf("build failed: %w", err)
		}
	}

	// Success: mark as built
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusBuilt
	}); err != nil {
		return app, fmt.Errorf("build succeeded but failed to update status: %w", err)
	}

	// Return the refreshed app
	updated, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("build succeeded but failed to fetch updated app: %w", err)
	}
	return updated, nil
}

// Helper functions
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func respondJSON(w http.ResponseWriter, status int, app any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(app)
}

func logStreamJSON(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(status)
}
