package handlers

import "net/http"

// HealthResponse matches the exact contract specified in the assignment:
// {"status": "ok"}.
type HealthResponse struct {
	Status string `json:"status"`
}

// Health handles GET /health. It is intentionally unauthenticated and has
// no dependency on the store, so it stays reliable even if downstream
// dependencies degrade — which is exactly what a health check should do.
func Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}
