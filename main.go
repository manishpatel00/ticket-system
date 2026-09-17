// Command ticket-system runs the Backend Intern assignment's REST API:
// a JWT-authenticated ticket system where each user can only see and
// modify their own tickets.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/evabharat/ticket-system/internal/auth"
	"github.com/evabharat/ticket-system/internal/handlers"
	"github.com/evabharat/ticket-system/internal/middleware"
	"github.com/evabharat/ticket-system/internal/store"
)

// config is loaded once at startup from environment variables (with
// sensible defaults for local development), per the .env.example contract.
type config struct {
	port      string
	jwtSecret string
	jwtTTL    time.Duration
}

func loadConfig() config {
	cfg := config{
		port:      getEnv("PORT", "8080"),
		jwtSecret: getEnv("JWT_SECRET", ""),
		jwtTTL:    24 * time.Hour,
	}

	if raw := os.Getenv("JWT_TTL_HOURS"); raw != "" {
		if hours, err := strconv.Atoi(raw); err == nil && hours > 0 {
			cfg.jwtTTL = time.Duration(hours) * time.Hour
		}
	}

	if cfg.jwtSecret == "" {
		// Fail fast: an empty JWT secret would make every token trivially
		// forgeable. Better to refuse to start than to run insecurely.
		log.Fatal("JWT_SECRET environment variable is required (see .env.example)")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func buildRouter(s store.Store, jwtManager *auth.Manager) http.Handler {
	mux := http.NewServeMux()

	authHandler := handlers.NewAuthHandler(s, jwtManager)
	ticketHandler := handlers.NewTicketHandler(s)
	requireAuth := middleware.RequireAuth(jwtManager)

	// Public routes.
	mux.HandleFunc("GET /{$}", handlers.Health) // root path
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/login", authHandler.Login)

	// Protected routes — wrapped individually with requireAuth so the
	// route table above stays a single readable list of every endpoint,
	// public or private, with its exact method and path.
	mux.Handle("POST /tickets", requireAuth(http.HandlerFunc(ticketHandler.Create)))
	mux.Handle("GET /tickets", requireAuth(http.HandlerFunc(ticketHandler.List)))
	mux.Handle("GET /tickets/{id}", requireAuth(http.HandlerFunc(ticketHandler.Get)))
	mux.Handle("PATCH /tickets/{id}/status", requireAuth(http.HandlerFunc(ticketHandler.UpdateStatus)))

	return middleware.Logging(mux)
}

func main() {
	cfg := loadConfig()

	jwtManager, err := auth.NewManager(cfg.jwtSecret, cfg.jwtTTL)
	if err != nil {
		log.Fatalf("failed to initialize JWT manager: %v", err)
	}

	memStore := store.NewMemoryStore()
	router := buildRouter(memStore, jwtManager)

	srv := &http.Server{
		Addr:         ":" + cfg.port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run the server in a goroutine so main() can block on a shutdown
	// signal below (graceful shutdown, not required by the brief but
	// good practice for anything that gets deployed).
	go func() {
		log.Printf("ticket-system listening on :%s", cfg.port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
