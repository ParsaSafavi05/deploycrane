package api

import (
	"net/http"

	"github.com/ParsaSafavi05/deploycrane/internal/api/handlers"
	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/health"
	"github.com/ParsaSafavi05/deploycrane/internal/service"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
	"github.com/moby/moby/client"
)

type Server struct {
	handler       *handlers.Handler
	healthHandler http.Handler
}

func NewServer(dockerClient client.APIClient, s store.Store, pm *docker.PortManager, cfg config.Config, watcher *docker.ContainerWatcher) *Server {
	dockerCheck := docker.NewHealthChecker(dockerClient)
	storeCheck := store.NewHealthChecker(s)
	healthHandler := health.NewHandler(dockerCheck, storeCheck)

	appService := service.NewAppService(s, dockerClient, watcher, pm, cfg)
	handler := handlers.NewHandler(s, dockerClient, appService, cfg)

	return &Server{
		handler:       handler,
		healthHandler: healthHandler,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/health", s.healthHandler)

	mux.HandleFunc("GET /containers", s.handler.HandleListContainers)
	mux.HandleFunc("GET /containers/{id}", s.handler.HandleGetContainer)
	mux.HandleFunc("POST /containers/start", s.handler.HandleStartContainer)
	mux.HandleFunc("POST /containers/{id}/stop", s.handler.HandleStopContainer)

	mux.HandleFunc("GET /apps", s.handler.HandleListApps)
	mux.HandleFunc("GET /apps/{id}", s.handler.HandleGetApp)
	mux.HandleFunc("POST /apps", s.handler.HandleCreateApp)
	mux.HandleFunc("POST /apps/{id}/clone", s.handler.HandleCloneApp)
	mux.HandleFunc("POST /apps/{id}/build", s.handler.HandleBuildApp)
	mux.HandleFunc("POST /apps/{id}/start", s.handler.HandleStartApp)
	mux.HandleFunc("POST /apps/{id}/stop", s.handler.HandleStopApp)
	mux.HandleFunc("POST /apps/{id}/deploy", s.handler.HandleDeployApp)
	mux.HandleFunc("DELETE /apps/{id}", s.handler.HandleDeleteApp)
	mux.HandleFunc("PUT /apps/{id}", s.handler.HandleUpdateApp)

	return mux
}