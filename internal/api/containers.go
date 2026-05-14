package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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

	// Call the docker package function
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