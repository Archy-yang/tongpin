package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/Archy-yang/tongpin/backend/internal/config"
	"github.com/Archy-yang/tongpin/backend/internal/platform/database"
	"github.com/Archy-yang/tongpin/backend/internal/server"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	srv := server.New(cfg, pool)
	srv.Start(ctx)
	log.Printf("server listening on :%s (env=%s)", cfg.Port, cfg.Env)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	select {
	case err := <-errCh:
		log.Fatalf("server: %v", err)
	case <-ctx.Done():
		log.Println("shutting down")
	}
}
