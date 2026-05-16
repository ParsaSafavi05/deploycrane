package api

import (
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
	Name    string `json:"name"`
	RepoURL string `json:"repo_url"`
	Deploy  string `json:"deploy"`
}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apps)
}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(app)

}

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
	}

	// Basic URL validation
	if !strings.HasPrefix(in.RepoURL, "http://") && !strings.HasPrefix(in.RepoURL, "https://") {
		writeError(w, http.StatusBadRequest, "repo_url must be a valid HTTP(S) URL")
		return
	}

	// Build app model

	app := model.App{
		ID:        uuid.New().String(),
		Name:      in.Name,
		RepoURL:   in.RepoURL,
		Status:    model.StatusCreated,
		CreatedAt: time.Now(),
	}

	// Store the app

	if err := s.store.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save app")
		return
	}

	// Otherwise, trigger the full deploy pipeline.
	if strings.EqualFold(in.Deploy, "no") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(app)
		return
	}

	// Auto-deploy: the function streams progress as Server-Sent Events.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusCreated) // Note: SSE stream starting

	// Call the core deploy logic, which is now shared
	s.deployApp(w, r, app)

}

func (s *Server) handleCloneApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Read-only check
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch app")
		return
	}

	if app.Status != model.StatusCreated && app.Status != model.StatusFailed {
		writeError(w, http.StatusConflict, "app is not in a cloneable state (must be 'created')")
		return
	}

	// Set status to cloning
	if err := s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusCloning
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app status")
		return
	}

	// Perform the clone
	log.Printf("cloning app %s from %s - id: %s", app.Name, app.RepoURL, id)
	clonePath := fmt.Sprintf("/tmp/deploycrane/app-%s", app.ID)
	if err := git.Clone(r.Context(), app.RepoURL, clonePath); err != nil {
		// Failed – atomically mark as failed
		_ = s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		writeError(w, http.StatusInternalServerError, "clone failed: "+err.Error())
		return
	}

	log.Printf("app %s was cloned successfuly - id: %s", app.Name, id)

	// Success – atomically set mark as cloned
	if err := s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusCloned
		a.ClonePath = clonePath
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app after clone")
		return
	}

	// Return the updated app
	app, err = s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "clone succeeded but could not fetch updated state")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(app)
}

func (s *Server) handleBuildApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Read-only check
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if app.Status == model.StatusBuilt {
		http.Error(w, "app already built", http.StatusBadRequest)
		return
	}
	if app.Status != model.StatusCloned {
		http.Error(w, "app not ready for build", http.StatusBadRequest)
		return
	}

	// Set status to building
	if err := s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusBuilding
	}); err != nil {
		http.Error(w, "failed to update app", http.StatusInternalServerError)
		return
	}

	imageName := "deploycrane-" + app.Name + ":latest"

	// Build the image
	body, err := docker.ImageBuild(r.Context(), s.dockerClient, app.ClonePath, imageName)
	if err != nil {
		// Build failed – atomically set status to failed
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()

	// Stream build logs
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Stream parsed logs as Server-Side-Events

	err = docker.StreamBuildLogs(w, body)

	// Update value based on failure/success
	if err != nil {
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		log.Printf("build for %s failed: %v", id, err)
	}
	// Build succeeded – atomically set status to built
	log.Printf("app %s was built successfuly - tag: %s", app.Name, imageName)
	s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusBuilt
	})
}

func (s *Server) handleStartApp(w http.ResponseWriter, r *http.Request) {

	id := r.PathValue("id")

	// Readonly check
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if app.Status == model.StatusStarting {
		http.Error(w, "app already starting", http.StatusBadRequest)
		return
	}
	if app.Status == model.StatusRunning {
		http.Error(w, "app already running", http.StatusBadRequest)
		return
	}
	if app.Status != model.StatusBuilt && app.Status != model.StatusFailed {
		http.Error(w, "app is not ready to start", http.StatusBadRequest)
		return
	}

	// Set status to starting
	if err := s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusStarting
	}); err != nil {
		http.Error(w, "failed to start app", http.StatusInternalServerError)
		return
	}

	imageName := "deploycrane-" + app.Name + ":latest"
	// Start container
	containerID, err := docker.StartContainer(r.Context(), s.dockerClient, imageName)
	if err != nil {
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		writeError(w, http.StatusInternalServerError, "failed to start container: "+err.Error())
		return
	}

	// Start succeeded - save container id and set status to running
	log.Printf("app %s started successfuly - container id: %v", app.Name, app.ContainerID)
	if err := s.store.Update(r.Context(), id, func(a *model.App) {
		a.Status = model.StatusRunning
		a.ContainerID = containerID
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "container started but failed to update app")
		return
	}

	app, _ = s.store.Get(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(app)
}

func (s *Server) handleDeployApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	s.deployApp(w, r, app)
}

// deployApp runs the full deployment pipeline and streams progress to w.
// It returns the final app state or an error.
func (s *Server) deployApp(w io.Writer, r *http.Request, app model.App) (model.App, error) {
	id := app.ID

	// Validate state
	switch app.Status {
	case model.StatusCreated, model.StatusFailed,
		model.StatusCloned, model.StatusBuilt, model.StatusStopped:
		// allowed
	case model.StatusRunning:
		return app, fmt.Errorf("app is already running")
	default:
		return app, fmt.Errorf("app is in a transitional state")
	}

	imageName := "deploycrane-" + app.Name + ":latest"

	// ---- Step 1: Clone ----
	if app.Status == model.StatusCreated || app.Status == model.StatusFailed {
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusCloning
		})

		clonePath := fmt.Sprintf("/tmp/deploycrane/app-%s", app.ID)
		if err := git.Clone(r.Context(), app.RepoURL, clonePath); err != nil {
			s.store.Update(r.Context(), id, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			fmt.Fprintf(w, "event: error\ndata: clone failed: %s\n\n", err.Error())
			return app, err
		}

		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusCloned
			a.ClonePath = clonePath
		})
		fmt.Fprintf(w, "data: Repository cloned successfully\n\n")
	}

	// Refresh app
	app, _ = s.store.Get(r.Context(), id)

	// ---- Step 2: Build ----
	if app.Status != model.StatusBuilt && app.Status != model.StatusStopped {
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusBuilding
		})

		body, err := docker.ImageBuild(r.Context(), s.dockerClient, app.ClonePath, imageName)
		if err != nil {
			s.store.Update(r.Context(), id, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			fmt.Fprintf(w, "event: error\ndata: build failed: %s\n\n", err.Error())
			return app, err
		}
		defer body.Close()

		if err := docker.StreamBuildLogs(w, body); err != nil {
			s.store.Update(r.Context(), id, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			return app, err
		}

		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusBuilt
		})
	}

	// ---- Step 3: Start ----
	app, _ = s.store.Get(r.Context(), id)
	if app.Status != model.StatusRunning {
		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusStarting
		})

		containerID, err := docker.StartContainer(r.Context(), s.dockerClient, imageName)
		if err != nil {
			s.store.Update(r.Context(), id, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			fmt.Fprintf(w, "event: error\ndata: start failed: %s\n\n", err.Error())
			return app, err
		}

		s.store.Update(r.Context(), id, func(a *model.App) {
			a.Status = model.StatusRunning
			a.ContainerID = containerID
		})
	}

	fmt.Fprintf(w, "event: complete\ndata: deploy finished — app is running\n\n")

	app, _ = s.store.Get(r.Context(), id)
	return app, nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
