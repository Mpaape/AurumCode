package main

import (
	"aurumcode/internal/git/webhook"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Load configuration
	cfg := LoadConfig()

	log.Printf("AurumCode Server - Starting on port %s", cfg.Port)
	if cfg.EnableDebugLogs {
		log.Println("Debug logging enabled")
	}

	// Create idempotency cache (15 minute TTL, no max size)
	idempotencyCache := createIdempotencyCache()

	// Create router
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/healthz", HealthHandler)
	mux.HandleFunc("/metrics", MetricsHandler)
	mux.HandleFunc("/webhook/github", WebhookHandler(cfg, idempotencyCache))

	// Apply middleware
	handler := RequestIDMiddleware(
		LoggingMiddleware(
			RecoveryMiddleware(mux),
		),
	)

	// Create server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeout)*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
		os.Exit(1)
	}

	log.Println("Server stopped")
	fmt.Println("Goodbye!")
}

func createIdempotencyCache() *webhook.IdempotencyCache {
	return webhook.NewIdempotencyCache(15*time.Minute, 0)
}
