package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// errorResponse is the consistent JSON shape returned for every error
// (validation, auth, not-found, server) so API consumers can always look
// for the same "error" key regardless of status code.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON marshals v as JSON with the given status code. Marshal errors
// are logged server-side; the client already has the status code sent,
// so at that point we just log — writing a second body would be invalid.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error encoding JSON response: %v", err)
	}
}

// writeError writes a standard {"error": msg} JSON body with the given
// HTTP status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
