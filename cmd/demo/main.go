package main

import (
	"log/slog"
	"os"

	"miren.dev/trifle"
)

func main() {
	// Create a logger with important keys highlighted in yellow
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}, trifle.WithImportantKeys("user_id", "error", "request_id", "status"))

	logger := slog.New(handler)

	// Demonstrate various log levels with important and normal keys
	logger.Debug("Starting application",
		"version", "1.0.0",
		"environment", "development",
	)

	logger.Info("User authentication attempt",
		"user_id", "12345", // Highlighted
		"username", "john.doe",
		"request_id", "req-789", // Highlighted
		"ip_address", "192.168.1.1",
	)

	logger.Warn("Rate limit approaching",
		"user_id", "12345", // Highlighted
		"requests_made", 95,
		"limit", 100,
		"window", "1h",
	)

	logger.Error("Failed to process payment",
		"error", "payment gateway timeout", // Highlighted
		"user_id", "12345", // Highlighted
		"amount", 99.99,
		"currency", "USD",
		"status", "failed", // Highlighted
	)

	// Demonstrate with groups
	requestLogger := logger.WithGroup("request").With("request_id", "req-123")
	requestLogger.Info("Processing request",
		"method", "POST",
		"path", "/api/users",
		"status", "200", // Highlighted even in groups
		"duration_ms", 145,
	)
}
