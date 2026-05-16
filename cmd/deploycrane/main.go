package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/api"
	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/store"
)

func main() {
	// Create docker client
	cli, err := docker.NewClient()
	if err != nil {
		log.Fatalf("failed to create docker client: %v", err)
	}

	// Create in memory tore
	storeInstance, err := store.NewSQLiteStore("deploycrane.db")
	if err != nil {
		log.Fatalf("failed to open sqlite store: %v", err)
	}

	// Create server with all the dependencies
	server := api.NewServer(cli, storeInstance)
	handler := server.Handler()

	// Configure the HTTP server with timeouts
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Listening on port 8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed{
			log.Fatalf("server error: %v", err)
		}
		log.Println("HTTP server stopped")

	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Recieved signal %v shutting down gracefully...", sig)

	// Give outstanding requests 30 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err = srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Close the database connection
	if err = storeInstance.Close(); err != nil {
		log.Printf("Error closing store: %v", err)
	}

	log.Println("Server stopped cleanly")
}
