// Package middleware holds HTTP middleware, currently just JWT
// authentication.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/evabharat/ticket-system/internal/auth"
)

type contextKey string

const userIDContextKey contextKey = "userID"

// RequireAuth returns middleware that enforces a valid
// "Authorization: Bearer <token>" header, verifies the JWT, and injects
// the authenticated user's ID into the request context for downstream
// handlers to read via UserIDFromContext.
func RequireAuth(jwtManager *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				writeAuthError(w, "missing Authorization header")
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				writeAuthError(w, "Authorization header must use Bearer scheme")
				return
			}
			token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
			if token == "" {
				writeAuthError(w, "missing bearer token")
				return
			}

			claims, err := jwtManager.ParseAndVerify(token)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrExpiredToken):
					writeAuthError(w, "token expired")
				default:
					writeAuthError(w, "invalid token")
				}
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the authenticated user's ID, set by
// RequireAuth. ok is false if called on a request that never passed
// through the auth middleware.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDContextKey).(string)
	return id, ok
}

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}
