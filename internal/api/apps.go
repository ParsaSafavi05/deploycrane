package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/google/uuid"
)

type input struct {
	Name string `json:"name"`
	RepoURL string `json:"repo_url"`
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request)  {
	id := r.PathValue("id")
	
	app, err := s.store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound){
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
	if err := json.NewDecoder(r.Body).Decode(&in); err!= nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return 
	}

	// Validate required fields
	in.Name = strings.TrimSpace(in.Name)
	in.RepoURL = strings.TrimSpace(in.RepoURL)

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
		ID: uuid.New().String(),
		Name: in.Name,
		RepoURL: in.RepoURL,
		Status: model.StatusCreated,
		CreatedAt: time.Now(),
	}

	// Store the app

	if err := s.store.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save app")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(app)

	
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}