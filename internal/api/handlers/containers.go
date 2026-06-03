package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(containers)
}

func (h *Handler) HandleGetContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.InspectContainer(r.Context(), h.dockerClient, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(info)
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
		writeError(w, http.StatusInternalServerError, "failed to start container")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"container_id": containerID})
}

func (h *Handler) HandleStopContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.StopContainer(r.Context(), h.dockerClient, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(info)
}