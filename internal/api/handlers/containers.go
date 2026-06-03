package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/middleware"
)

func (h *Handler) HandleListContainers(w http.ResponseWriter, r *http.Request) {
	all := false
	if allParam := r.URL.Query().Get("all"); allParam != "" {
		var err error
		all, err = strconv.ParseBool(allParam)
		if err != nil {
			http.Error(w, "Invalid 'all' parameter, must be true/false", http.StatusBadRequest)
			return
		}
	}

	containers, err := docker.ListContainers(r.Context(), h.dockerClient, all)
	if err != nil {
		http.Error(w, "Failed to list containers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, containers)
}

func (h *Handler) HandleGetContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.InspectContainer(r.Context(), h.dockerClient, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	respondJSON(w, http.StatusOK, info)
}

type startInput struct {
	Image string `json:"image"`
}

func (h *Handler) HandleStartContainer(w http.ResponseWriter, r *http.Request) {
	var in startInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	in.Image = strings.TrimSpace(in.Image)
	if in.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}

	containerPort := 8199
	hostPort := 8199

	portMappings := map[int]int{containerPort: hostPort}
	containerID, err := docker.StartContainer(r.Context(), h.dockerClient, in.Image, portMappings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start container: "+err.Error())
		return
	}

	if containerID == "" {
		writeError(w, http.StatusInternalServerError, "failed to start container: empty container ID")
		return
	}

	middleware.ReqLog(r).Info("container started successfully", "container_id", containerID)
	respondJSON(w, http.StatusCreated, map[string]string{"container_id": containerID})
}

func (h *Handler) HandleStopContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.StopContainer(r.Context(), h.dockerClient, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	middleware.ReqLog(r).Info("container stopped successfully", "container_id", id)
	respondJSON(w, http.StatusOK, info)
}