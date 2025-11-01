package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request-id"

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has an ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to context
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		// Add to response headers
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs incoming requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get request ID from context
		requestID := r.Context().Value(requestIDKey)
		if requestID == nil {
			requestID = "unknown"
		}

		// Create response writer wrapper to capture status
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Log request
		log.Printf("[%s] %s %s - Started", requestID, r.Method, r.URL.Path)

		// Process request
		next.ServeHTTP(rw, r)

		// Log completion
		duration := time.Since(start)
		log.Printf("[%s] %s %s - Completed %d in %v", requestID, r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// RecoveryMiddleware recovers from panics and logs them
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := r.Context().Value(requestIDKey)
				if requestID == nil {
					requestID = "unknown"
				}

				log.Printf("[%s] PANIC: %v", requestID, err)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
