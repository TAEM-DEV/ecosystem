package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/taem-dev/ecosystem/internal/collections"
	"github.com/taem-dev/ecosystem/internal/handlers"
	"github.com/taem-dev/ecosystem/internal/qdrant"
)

func main() {
	// Read environment variables
	qdrantURL := envOrDefault("QDRANT_URL", "http://localhost:6333")
	qdrantAPIKey := os.Getenv("QDRANT_API_KEY")
	port := envOrDefault("PORT", "8765")
	freshnessHours := envIntOrDefault("FRESHNESS_HOURS", 24)
	forceStaleHours := envIntOrDefault("FORCE_STALE_HOURS", 168)

	// Create Qdrant client
	qClient := qdrant.NewClient(qdrantURL, qdrantAPIKey)

	// Init collections (idempotent)
	log.Println("initializing qdrant collections...")
	if err := collections.InitAll(qClient); err != nil {
		log.Printf("WARNING: failed to initialize collections: %v", err)
		log.Println("service will start but Qdrant may be unavailable")
	} else {
		log.Println("qdrant collections initialized successfully")
	}

	// Configure repo surfaces
	surfaceCfg := handlers.RepoSurfacesConfig{
		FreshnessHours:  freshnessHours,
		ForceStaleHours: forceStaleHours,
	}

	// Register handlers using http.ServeMux with manual path routing
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", handlers.HealthHandler(qClient))

	// Repo surfaces — path pattern /api/repo_surfaces/{org}/{repo}[/stale]
	mux.HandleFunc("/api/repo_surfaces/", handlers.RepoSurfacesHandler(qClient, surfaceCfg))

	// Lessons learned
	mux.HandleFunc("/api/lessons/", handlers.LessonsHandler(qClient))
	mux.HandleFunc("/api/lessons", handlers.LessonsHandler(qClient))

	// Constraints
	mux.HandleFunc("/api/constraints/", handlers.ConstraintsHandler(qClient))

	// Mission memory
	mux.HandleFunc("/api/mission_memory/", handlers.MissionMemoryHandler(qClient))
	mux.HandleFunc("/api/mission_memory", handlers.MissionMemoryHandler(qClient))

	// Wiring patterns
	mux.HandleFunc("/api/wiring_patterns/", handlers.WiringPatternsHandler(qClient))
	mux.HandleFunc("/api/wiring_patterns", handlers.WiringPatternsHandler(qClient))

	// Domain knowledge (ADR-009a: field intelligence from refexplorer)
	mux.HandleFunc("/api/domain_knowledge/", handlers.DomainKnowledgeHandler(qClient))
	mux.HandleFunc("/api/domain_knowledge", handlers.DomainKnowledgeHandler(qClient))

	// Create server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("ecosystem service starting on :%s", port)
		log.Printf("qdrant: %s", qdrantURL)
		log.Printf("freshness: %dh, force-stale: %dh", freshnessHours, forceStaleHours)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-done
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("ecosystem service stopped")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
