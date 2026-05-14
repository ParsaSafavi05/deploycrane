package api

import (
	"net/http"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/health"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/moby/moby/client"
)

type Server struct{
	dockerClient client.APIClient
	store store.Store
	healthHandler http.Handler
}

func NewServer(dockerClient client.APIClient, s store.Store) *Server  {
	// Build health checkers
	dockerCheck := docker.NewHealthChecker(dockerClient)
	storeCheck := store.NewHealthChecker(s)

	healthHandler := health.NewHandler(dockerCheck, storeCheck)

	return &Server{
		dockerClient: dockerClient,
		store: s,
		healthHandler: healthHandler,
	}
}

// Returns HTTP handler with all routes mounted

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", s.healthHandler)

	// ...FUTURE_ROUTES_HERE...

	return mux
}