package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/ParsaSafavi05/deploycrane/internal/api"
	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/logging"
	"github.com/ParsaSafavi05/deploycrane/internal/middleware"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
)

func main() {
	// Initialize logging
	logging.Init("info", "deploycrane", "dev")
	
	// Load config
	cfg, err := config.Load()
	if err != nil {
		logging.Fatal("load config failed", "error", err)
	}

	// Create docker client
	cli, err := docker.NewClient()
	if err != nil {
		logging.Fatal("failed to create docker client", "error", err)
	}

	// Create sqlite store
	storeInstance, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		logging.Fatal("failed to open sqlite store", "error", err)
	}

	// Setup container watcher
	watcher := docker.NewContainerWatcher(cli, storeInstance)

	// Create a context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Restore previous mappings
	if err := watcher.RestoreMappings(ctx); err != nil {
		logging.Fatal("restore watcher mapping", "error", err)
	}
	go watcher.WatchLoop(ctx)

	// Create the port manager
	pm := docker.NewPortManager(cfg.PortAllocationMin, cfg.PortAllocationMax)

	// Create server with all the dependencies
	server := api.NewServer(cli, storeInstance, pm, *cfg, watcher)
	handler := middleware.CORS(cfg.CORSOrigins)(server.Handler())

	// Configure the HTTP server with timeouts
	srv := &http.Server{
		Addr:         cfg.ServerAddr + ":" + cfg.ListenPort,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Start HTTP server
	go func() {
		logging.Info("server starting",
		"addr", cfg.ServerAddr,
		"port", cfg.ListenPort,
	)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Fatal("server error", "error", err)
		}
		logging.Info("HTTP server stopped")
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logging.Info("shutdown signal received")
	logging.Info("shutting down gracefully")

	// Give outstanding requests 30 seconds to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logging.Error("server forced to shutdown", "error", err)
	}

	// Close the database connection
	if err := storeInstance.Close(); err != nil {
		logging.Error("error closing store", "error", err)
	}

	logging.Info("server stopped cleanly")
}
