package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/ParsaSafavi05/deploycrane/internal/api"
	"github.com/ParsaSafavi05/deploycrane/internal/config"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
)

func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Create docker client
	cli, err := docker.NewClient()
	if err != nil {
		log.Fatalf("failed to create docker client: %v", err)
	}

	// Create sqlite store
	storeInstance, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open sqlite store: %v", err)
	}

	// Setup container watcher
	watcher := docker.NewContainerWatcher(cli, storeInstance)

	// Create a context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Restore previous mappings
	if err := watcher.RestoreMappings(ctx); err != nil {
		log.Fatalf("restore watcher mappings: %v", err)
	}
	go watcher.WatchLoop(ctx) 

	// Create the port manager
	pm := docker.NewPortManager(cfg.PortAllocationMin, cfg.PortAllocationMax)

	// Create server with all the dependencies
	server := api.NewServer(cli, storeInstance, pm, *cfg, watcher)
	handler := corsMiddleware(server.Handler())

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
		log.Printf("Listening on port %v", cfg.ListenPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
		log.Println("HTTP server stopped")
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Received shutdown signal, shutting down gracefully...")

	// Give outstanding requests 30 seconds to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close the database connection
	if err := storeInstance.Close(); err != nil {
		log.Printf("Error closing store: %v", err)
	}

	log.Println("Server stopped cleanly")
}
