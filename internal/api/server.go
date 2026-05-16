package api

import (
	"net/http"

	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/health"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/moby/moby/client"
)

type Server struct {
	dockerClient  client.APIClient
	store         store.Store
	healthHandler http.Handler
	cfg           config.Config
}

func NewServer(dockerClient client.APIClient, s store.Store, cfg config.Config) *Server {
	// Build health checkers
	dockerCheck := docker.NewHealthChecker(dockerClient)
	storeCheck := store.NewHealthChecker(s)

	healthHandler := health.NewHandler(dockerCheck, storeCheck)

	return &Server{
		dockerClient:  dockerClient,
		store:         s,
		healthHandler: healthHandler,
		cfg:           cfg,
	}
}

// Returns HTTP handler with all routes mounted

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check handlers
	mux.Handle("/health", s.healthHandler)

	// Container handlers
	mux.HandleFunc("GET /containers/{id}", s.handleGetContainer)

	mux.HandleFunc("POST /containers/start", s.handleStartContainer)
	mux.HandleFunc("POST /containers/{id}/stop", s.handleStopContainer)

	// App handlers
	mux.HandleFunc("GET /apps", s.handleListApps)
	mux.HandleFunc("GET /apps/{id}", s.handleGetApp)
	mux.HandleFunc("GET /containers", s.handleListContainers)

	mux.HandleFunc("POST /apps", s.handleCreateApp)
	mux.HandleFunc("POST /apps/{id}/clone", s.handleCloneApp)
	mux.HandleFunc("POST /apps/{id}/build", s.handleBuildApp)
	mux.HandleFunc("POST /apps/{id}/start", s.handleStartApp)
	mux.HandleFunc("POST /apps/{id}/deploy", s.handleDeployApp)

	return mux
}
