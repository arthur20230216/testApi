package main

import (
	"log"
	"net/http"
	"time"

	"modelprobe/backend/internal/config"
	"modelprobe/backend/internal/handler"
	"modelprobe/backend/internal/repository"
	"modelprobe/backend/internal/server"
	"modelprobe/backend/internal/service"
)

func main() {
	cfg := config.Load()

	repo, err := repository.NewPostgresRepository(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("init repository: %v", err)
	}
	defer repo.Close()

	probeService := service.NewProbeService(cfg.ProbeTimeout)
	probeHandler := handler.NewProbeHandler(repo, probeService)
	router := server.NewRouter(cfg, probeHandler)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("model-probe backend listening on http://localhost:%s", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
