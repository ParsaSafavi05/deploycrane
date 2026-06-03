package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/git"
	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/sse"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/google/uuid"
)

type input struct {
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	Deploy        bool `json:"deploy"`
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

	// Stream the creation flow as SSE
	logStreamJSON(w, http.StatusCreated)
	sse.WriteEvent(w, "endpoint", "SSE create endpoint active")

	// If deploy is false, just return
	if !in.Deploy {
		if payload, err := json.Marshal(app); err == nil {
			sse.WriteEvent(w, "app", string(payload))
		}
		sse.WriteEvent(w, "complete", "app created successfully")
		return
	}

	// If deploy is true, continue to deploy
	sse.WriteEvent(w, "endpoint", "SSE deploy endpoint active")
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

	// Stream progress via SSE
	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE clone endpoint active")

	// Run the shared clone logic
	updatedApp, err := s.cloneApp(r.Context(), app, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
		return
	}

	log.Printf("app %s cloned successfully - id: %s", updatedApp.Name, updatedApp.ID)
	sse.WriteEvent(w, "complete", "clone finished successfully")
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
	sse.WriteEvent(w, "endpoint", "SSE build endpoint active")

	// Call shared build logic
	updatedApp, err := s.buildApp(r.Context(), app, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
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
		writeError(w, http.StatusBadRequest, "app already started")
		return
	}
	if app.Status != model.StatusBuilt && app.Status != model.StatusFailed && app.Status != model.StatusStopped {
		writeError(w, http.StatusBadRequest, "app is not ready to start")
		return
	}

	containerPort := app.ContainerPort
	if containerPort <= 0 {
		// Fall back to default container port
		containerPort = s.cfg.ContainerPort
	}

	// Stream progress via SSE
	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE start endpoint active")
	updatedApp, err := s.startAppWithProgress(r.Context(), app, containerPort, app.HostPort, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
		return
	}
	log.Printf("app %s started successfuly - container id: %v", updatedApp.Name, updatedApp.ContainerID)
	sse.WriteEvent(w, "complete", fmt.Sprintf("app started successfully — container id: %s", updatedApp.ContainerID))
}

func (s *Server) handleStopApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Fetch current app state
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	updatedApp, err := s.stopApp(r.Context(), app)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("app %s stopped succesfully - app id %v", updatedApp.Name, updatedApp.ID)
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
	sse.WriteEvent(w, "endpoint", "SSE deploy endpoint active")
	s.deployApp(w, r, app)
}

func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	app, err := s.store.Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// 1Stop + remove container first (runtime cleanup)
	if app.ContainerID != "" {
		if err := docker.StopAndRemoveContainer(ctx, s.dockerClient, app.ContainerID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove container")
			return
		}
	}

	// Remove filesystem
	if app.ClonePath != "" {
		if err := os.RemoveAll(app.ClonePath); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove app files")
			return
		}
	}

	// Only now delete DB record (final commit point)
	if err := s.store.Delete(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete app")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "app deleted successfully",
	})
}

// deployApp runs the full deployment pipeline and streams progress
func (s *Server) deployApp(w io.Writer, r *http.Request, app model.App) (model.App, error) {
	id := app.ID

	// Validate state
	switch app.Status {
	case model.StatusCreated, model.StatusFailed,
		model.StatusCloned, model.StatusBuilt, model.StatusStopped:
	case model.StatusRunning:
		return app, fmt.Errorf("app already deployed")
	default:
		return app, fmt.Errorf("app is in a transitional state")
	}

	// ---- Step 1: Clone ----
	if app.Status == model.StatusCreated || app.Status == model.StatusFailed {
		updatedApp, err := s.cloneApp(r.Context(), app, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("clone failed: %s", err.Error()))
			return app, err
		}
		app = updatedApp
		sse.WriteEvent(w, "", "Repository cloned successfully")
	}
	// Re-fetch the app (already done inside cloneApp, but we assign to `app`)
	app, _ = s.store.Get(r.Context(), id)

	// ---- Step 2: Build ----
	if app.Status != model.StatusBuilt && app.Status != model.StatusStopped {
		updatedApp, err := s.buildApp(r.Context(), app, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("build failed: %s", err.Error()))
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
		updatedApp, err := s.startAppWithProgress(r.Context(), app, containerPort, app.HostPort, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("start failed: %s", err.Error()))
			return app, err
		}
		app = updatedApp
	}

	sse.WriteEvent(w, "complete", "deploy finished — app is running")

	app, _ = s.store.Get(r.Context(), id)
	return app, nil
}

type ProgressReporter interface {
	Report(string)
}

type nopReporter struct{}

func (nopReporter) Report(string) {}

type sseReporter struct {
	w io.Writer
}

func (r sseReporter) Report(msg string) {
	sse.WriteEvent(r.w, "", msg)
}

// Wrapper for starting app with no progress
func (s *Server) startApp(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
) (model.App, error) {
	return s.startAppInternal(ctx, app, containerPort, userHostPort, nopReporter{})
}

// Wrapper for starting app with progress
func (s *Server) startAppWithProgress(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
	w io.Writer,
) (model.App, error) {
	flusher, ok := w.(http.Flusher)
	if !ok || flusher == nil {
		return s.startApp(ctx, app, containerPort, userHostPort)
	}

	return s.startAppInternal(
		ctx,
		app,
		containerPort,
		userHostPort,
		sseReporter{w: w},
	)
}

func (s *Server) startAppInternal(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
	reporter ProgressReporter,
) (model.App, error) {
	emit := func(msg string) {
		if reporter != nil {
			reporter.Report(msg)
		}
	}

	emit("validating ports...")

	// Validate container port
	if containerPort <= 0 || containerPort > 65535 {
		return app, fmt.Errorf("invalid container port %d", containerPort)
	}

	emit("allocating host port...")

	hostPort, err := s.reserveHostPort(userHostPort)
	if err != nil {
		return app, err
	}

	if userHostPort > 0 && userHostPort <= 65535 && hostPort != userHostPort {
		emit(fmt.Sprintf("requested host port %d was unavailable; using %d...", userHostPort, hostPort))
	}

	emit(fmt.Sprintf("starting container (port %d → %d)...", containerPort, hostPort))

	// Set status to starting
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusStarting
	}); err != nil {
		return app, fmt.Errorf("failed to start app: %w", err)
	}

	imageName := s.cfg.ImagePrefix + "-" + app.Name + ":latest"
	portMappings := map[int]int{containerPort: hostPort}

	containerID, err := docker.StartContainer(ctx, s.dockerClient, imageName, portMappings)
	if err != nil {
		if updateErr := s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		}); updateErr != nil {
			log.Printf("failed to mark app %s as failed after container start error: %v", app.ID, updateErr)
		}
		return app, fmt.Errorf("failed to start container: %w", err)
	}

	emit("container created, registering watcher...")

	s.watcher.RegisterApp(app.ID, containerID)

	emit("updating database...")

	// Start succeeded - save container id and set status to running
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusRunning
		a.ContainerID = containerID
		a.HostPort = hostPort
	}); err != nil {
		log.Printf("CRITICAL: container started but DB update failed for app %s: %v", app.ID, err)
		return app, fmt.Errorf("container started but failed to update status: %w", err)
	}

	updatedApp, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("failed to fetch updated app: %w", err)
	}

	return updatedApp, nil
}

// Helper for port reservation
func (s *Server) reserveHostPort(userHostPort int) (int, error) {
	userProvided := userHostPort > 0 && userHostPort <= 65535

	if userProvided {
		if s.pm.IsAvailable(userHostPort) {
			if err := s.pm.AllocateSpecific(userHostPort); err != nil {
				return 0, fmt.Errorf("failed to reserve port %d: %w", userHostPort, err)
			}
			return userHostPort, nil
		}

		allocated, err := s.pm.Allocate()
		if err != nil {
			return 0, fmt.Errorf("requested host port %d is unavailable and no free ports", userHostPort)
		}
		return allocated, nil
	}

	allocated, err := s.pm.Allocate()
	if err != nil {
		return 0, fmt.Errorf("no free ports")
	}

	return allocated, nil
}

// stopApp stops the container and updates the db
func (s *Server) stopApp(ctx context.Context, app model.App) (model.App, error) {

	// Validate state
	if app.Status != model.StatusRunning {
		return app, fmt.Errorf("app is not running")
	}

	if app.ContainerID == "" {
		return app, fmt.Errorf("app has no active container")
	}
	// Stop container
	_, err := docker.StopContainer(ctx, s.dockerClient, app.ContainerID)

	if err != nil {
		return app, fmt.Errorf("failed to stop container: %s", err)
	}

	if err = s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusStopped
	}); err != nil {
		return app, fmt.Errorf("failed to update app status: %s", err)
	}

	updatedApp, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("app stopped but failed to refetch the app: %s", err)
	}
	return updatedApp, nil
}

// cloneApp clones the repository for the given app and returns the updated app.
func (s *Server) cloneApp(ctx context.Context, app model.App, w io.Writer) (model.App, error) {
	// Validate state
	if app.Status == model.StatusCloned {
		return app, fmt.Errorf("app already cloned")
	}
	if app.Status == model.StatusCloning {
		return app, fmt.Errorf("app already cloning")
	}
	if app.Status != model.StatusCreated && app.Status != model.StatusFailed {
		return app, fmt.Errorf("app is not in a cloneable state (current: %s)", app.Status)
	}

	// Set status to cloning
	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusCloning
	}); err != nil {
		return app, fmt.Errorf("failed to set status to cloning: %w", err)
	}

	if w != nil {
		sse.WriteEvent(w, "", fmt.Sprintf("cloning repository from %s...", app.RepoURL))
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

	if w != nil {
		sse.WriteEvent(w, "", "clone complete")
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
	if app.Status == model.StatusBuilding {
		return app, fmt.Errorf("app already building")
	}
	if app.Status != model.StatusCloned && app.Status != model.StatusFailed{
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
		// Run a heartbeat in parallel to keep connection alive during long builds
		done := make(chan struct{})
		defer close(done)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ticker.C:
					sse.WriteEvent(w, "ping", "build in progress...")
				case <-done:
					return
				}
			}
		}()

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
	w.Header().Set("X-Accel-Buffering", "no")

	w.WriteHeader(status)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
