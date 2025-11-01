package main

import (
	"aurumcode/internal/git/webhook"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// HealthHandler handles health check requests
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// MetricsHandler handles metrics requests (placeholder)
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("# AurumCode Metrics\n\n# TODO: Implement Prometheus metrics\n"))
}

// WebhookHandler handles GitHub webhook requests
func WebhookHandler(cfg *ServerConfig, cache *webhook.IdempotencyCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(requestIDKey)

		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[%s] Failed to read webhook body: %v", requestID, err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to read request body",
			})
			return
		}
		defer r.Body.Close()

		// Validate signature
		signature := r.Header.Get("X-Hub-Signature-256")
		err = webhook.ValidateGitHubSignature(signature, body, cfg.WebhookSecret)
		if err != nil {
			if errors.Is(err, webhook.ErrMissingSignature) {
				log.Printf("[%s] Missing webhook signature", requestID)
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "missing signature",
				})
				return
			}

			if errors.Is(err, webhook.ErrInvalidSignature) || errors.Is(err, webhook.ErrMalformedSignature) {
				log.Printf("[%s] Invalid webhook signature: %v", requestID, err)
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid signature",
				})
				return
			}

			// Unknown error
			log.Printf("[%s] Signature validation error: %v", requestID, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "internal error",
			})
			return
		}

		// Parse event
		eventType := r.Header.Get("X-GitHub-Event")
		deliveryID := r.Header.Get("X-GitHub-Delivery")

		parser := webhook.NewGitHubEventParser()
		event, err := parser.Parse(eventType, deliveryID, signature, body)
		if err != nil {
			// Check if it's an unsupported event (return 204)
			if errors.Is(err, webhook.ErrUnsupportedEvent) {
				log.Printf("[%s] Unsupported event: %v", requestID, err)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Invalid payload
			log.Printf("[%s] Failed to parse event: %v", requestID, err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid event payload",
			})
			return
		}

		// Check for duplicate delivery
		if cache.SeenOrAdd(event.DeliveryID) {
			log.Printf("[%s] Duplicate delivery ID: %s - ignoring", requestID, event.DeliveryID)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "duplicate",
				"message": "delivery already processed",
			})
			return
		}

		// TODO: Process event (emit to channel/queue)

		log.Printf("[%s] Event parsed: type=%s repo=%s delivery=%s",
			requestID, event.EventType, event.Repo, event.DeliveryID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":     "received",
			"event_type": event.EventType,
			"repo":       event.Repo,
		})
	}
}
