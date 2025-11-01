package main

import (
	"os"
	"strconv"
)

// ServerConfig holds server configuration from environment
type ServerConfig struct {
	Port              string
	WebhookSecret     string
	GitHubToken       string
	OpenAIKey         string
	ShutdownTimeout   int
	EnableDebugLogs   bool
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *ServerConfig {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret == "" {
		webhookSecret = "dev-secret" // default for dev only
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	shutdownTimeout := 30
	if val := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			shutdownTimeout = parsed
		}
	}

	debugLogs := os.Getenv("DEBUG_LOGS") == "true"

	return &ServerConfig{
		Port:            port,
		WebhookSecret:   webhookSecret,
		GitHubToken:     githubToken,
		OpenAIKey:       openaiKey,
		ShutdownTimeout: shutdownTimeout,
		EnableDebugLogs: debugLogs,
	}
}
