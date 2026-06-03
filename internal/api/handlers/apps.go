package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/service"
	"github.com/ParsaSafavi05/deploycrane/internal/sse"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
)

type Handler struct {
	store        store.Store
	dockerClient client.APIClient
	appService   *service.AppService
	cfg          config.Config
}

func NewHandler(store store.Store, dockerClient client.APIClient, appService *service.AppService, cfg config.Config) *Handler {
	return &Handler{
		store:        store,
		dockerClient: dockerClient,
		appService:   appService,
		cfg:          cfg,
	}
}

type input struct {
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	Deploy        bool   `json:"deploy"`
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
}

func (h *Handler) HandleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list apps")
		return
	}
	if apps == nil {
		apps = []model.App{}
	}
	respondJSON(w, http.StatusOK, apps)
}

func (h *Handler) HandleGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
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

func (h *Handler) HandleCreateApp(w http.ResponseWriter, r *http.Request) {
	var in input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	in.RepoURL = strings.TrimSpace(in.RepoURL)

	if in.Name == "" || in.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "name and repo_url are required")
		return
	}
	if !strings.HasPrefix(in.RepoURL, "http://") && !strings.HasPrefix(in.RepoURL, "https://") {
		writeError(w, http.StatusBadRequest, "repo_url must be a valid HTTP(S) URL")
		return
	}

	app := model.App{
		ID:            uuid.New().String(),
		Name:          in.Name,
		RepoURL:       in.RepoURL,
		Status:        model.StatusCreated,
		CreatedAt:     time.Now(),
		ContainerPort: in.ContainerPort,
		HostPort:      in.HostPort,
	}

	if err := h.store.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save app")
		return
	}

	logStreamJSON(w, http.StatusCreated)
	sse.WriteEvent(w, "endpoint", "SSE create endpoint active")

	if !in.Deploy {
		if payload, err := json.Marshal(app); err == nil {
			sse.WriteEvent(w, "app", string(payload))
		}
		sse.WriteEvent(w, "complete", "app created successfully")
		return
	}

	sse.WriteEvent(w, "endpoint", "SSE deploy endpoint active")
	h.appService.DeployApp(w, r.Context(), app)
}

func (h *Handler) HandleCloneApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch app")
		return
	}

	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE clone endpoint active")

	updatedApp, err := h.appService.CloneApp(r.Context(), app, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
		return
	}

	log.Printf("app %s cloned successfully - id: %s", updatedApp.Name, updatedApp.ID)
	sse.WriteEvent(w, "complete", "clone finished successfully")
}

func (h *Handler) HandleBuildApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE build endpoint active")

	updatedApp, err := h.appService.BuildApp(r.Context(), app, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
		return
	}

	log.Printf("app %s built successfully - tag: %s", updatedApp.Name, h.cfg.ImagePrefix+"-"+updatedApp.Name+":latest")
}

func (h *Handler) HandleStartApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

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
		containerPort = h.cfg.ContainerPort
	}

	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE start endpoint active")

	updatedApp, err := h.appService.StartAppWithProgress(r.Context(), app, containerPort, app.HostPort, w)
	if err != nil {
		sse.WriteEvent(w, "error", err.Error())
		return
	}

	log.Printf("app %s started successfully - container id: %v", updatedApp.Name, updatedApp.ContainerID)
	sse.WriteEvent(w, "complete", fmt.Sprintf("app started successfully — container id: %s", updatedApp.ContainerID))
}

func (h *Handler) HandleStopApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	updatedApp, err := h.appService.StopApp(r.Context(), app)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("app %s stopped successfully - app id %v", updatedApp.Name, updatedApp.ID)
	respondJSON(w, http.StatusOK, updatedApp)
}

func (h *Handler) HandleDeployApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	logStreamJSON(w, http.StatusOK)
	sse.WriteEvent(w, "endpoint", "SSE deploy endpoint active")
	h.appService.DeployApp(w, r.Context(), app)
}

func (h *Handler) HandleDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	app, err := h.store.Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	if app.ContainerID != "" {
		if err := docker.StopAndRemoveContainer(ctx, h.dockerClient, app.ContainerID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove container")
			return
		}
	}

	if app.ClonePath != "" {
		if err := os.RemoveAll(app.ClonePath); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove app files")
			return
		}
	}

	if err := h.store.Delete(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete app")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "app deleted successfully",
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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