package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"expense-bot/config"
	"expense-bot/internal/repository/sqlite"
	"expense-bot/internal/server"
)

func main() {
	cfg := config.LoadServer()

	repo, err := sqlite.NewSQLiteRepository(cfg.DBPath)
	if err != nil {
		log.Fatalf("sqlite repository error: %v", err)
	}
	defer repo.Close()

	srv := server.NewServer(repo, server.Options{
		RateLimitRPS:   cfg.RateLimitRPS,
		RateLimitBurst: cfg.RateLimitBurst,
		AllowedOrigins: cfg.AllowedOrigins,
		Debug:          cfg.Debug,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx, cfg.Addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
	log.Printf("REST API stopped")
}
