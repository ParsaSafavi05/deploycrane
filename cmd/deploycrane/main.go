package main

import (
	"log"
	"net/http"

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

	// Get HTTP handler and start listening
	handler := server.Handler()
	log.Println("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}