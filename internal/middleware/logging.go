package middleware

import (
	"log"
	"net/http"
	"time"
)

// statusRecorder wraps a ResponseWriter to capture the status code that
// was actually written, since http.ResponseWriter doesn't expose it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Logging logs method, path, status code and latency for every request.
// It's minimal on purpose — enough to debug a deployed service via
// platform logs, without pulling in a structured-logging dependency.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}
