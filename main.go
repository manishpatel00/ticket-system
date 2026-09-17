// Command ticket-system runs the Backend Intern assignment's REST API:
// a JWT-authenticated ticket system where each user can only see and
// modify their own tickets.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
		// The assignment's Local Run Contract runs the container with
		// no -e flags at all (`docker run -p 8080:8080 ticket-system`),
		// so the service must come up healthy without JWT_SECRET set.
		// Refusing to start here would fail that exact contract.
		//
		// Trade-off, stated explicitly: we auto-generate a random,
		// process-lifetime-only secret instead of using any fixed
		// fallback value. This keeps tokens unforgeable by anyone
		// outside this running process (never a hardcoded/guessable
		// secret), at the cost of invalidating all issued tokens on
		// restart — acceptable for an in-memory store that already
		// loses all data on restart. For a real deployment, set
		// JWT_SECRET explicitly (see .env.example) so tokens and the
		// signing key stay stable across restarts/scaling.
		generated, err := generateRandomSecret(32)
		if err != nil {
			log.Fatalf("JWT_SECRET was not set and generating a fallback secret failed: %v", err)
		}
		cfg.jwtSecret = generated
		log.Println("WARNING: JWT_SECRET not set — using an auto-generated, process-lifetime-only secret. " +
			"Set JWT_SECRET explicitly for any deployment where tokens must survive a restart.")
	}

	return cfg
}

// generateRandomSecret returns a cryptographically random hex-encoded
// string of n random bytes, suitable as a JWT signing key.
func generateRandomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
