package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/ParsaSafavi05/deploycrane/internal/docker" // adjust import to your actual module
)

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	// Parse query parameter "all" (default: false)
	all := false
	if allParam := r.URL.Query().Get("all"); allParam != "" {
		var err error
		all, err = strconv.ParseBool(allParam)
		if err != nil {
			http.Error(w, "Invalid 'all' parameter, must be true/false", http.StatusBadRequest)
			return
		}
	}

	// Call t1e docker package function
	containers, err := docker.ListContainers(r.Context(), s.dockerClient, all)
	if err != nil {
		http.Error(w, "Failed to list containers: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		// Too late to change status, but log internally (optional)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (s *Server) handleGetContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.InspectContainer(r.Context(), s.dockerClient, id)

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

func (s *Server) handleStartContainer(w http.ResponseWriter, r *http.Request) {
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
	containerID, err := docker.StartContainer(r.Context(), s.dockerClient, in.Image, portMappings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start container")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"container_id": containerID})
}

func (s *Server) handleStopContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	info, err := docker.StopContainer(r.Context(), s.dockerClient, id)

	if err != nil {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(info)
}