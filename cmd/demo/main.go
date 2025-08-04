package main

import (
	"log/slog"
	"os"

	"miren.dev/trifle"
)

func main() {
	// Create a logger with:
	// - Critical keys (error, panic) highlighted in red
	// - Important keys (user_id, request_id, status) highlighted in yellow
	// - Context keys (request_id, session_id) shown before message
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	},
		trifle.WithCriticalKeys("error", "panic"),
		trifle.WithImportantKeys("user_id", "request_id", "status"),
		trifle.WithContextKey("request_id", "session_id", "trace_id"),
	)

	logger := slog.New(handler)

	// Demonstrate various log levels with important and normal keys
	logger.Debug("Starting application",
		"version", "1.0.0",
		"environment", "development",
	)

	logger.Info("User authentication attempt",
		"user_id", "12345", // Key highlighted in yellow
		"username", "john.doe",
		"request_id", "req-789", // Key highlighted in yellow and shown as context
		"session_id", "sess-abc", // Shown as context
		"ip_address", "192.168.1.1",
	)

	logger.Warn("Rate limit approaching",
		"user_id", "12345", // Key highlighted in yellow
		"requests_made", 95,
		"limit", 100,
		"window", "1h",
	)

	logger.Error("Failed to process payment",
		"error", "payment gateway timeout", // Key highlighted in yellow
		"user_id", "12345", // Key highlighted in yellow
		"amount", 99.99,
		"currency", "USD",
		"status", "failed", // Key highlighted in yellow
	)

	// Demonstrate with groups
	requestLogger := logger.WithGroup("request").With("request_id", "req-123")
	requestLogger.Info("Processing request",
		"method", "POST",
		"path", "/api/users",
		"status", "200", // Key highlighted in yellow even in groups
		"duration_ms", 145,
	)

	// Demonstrate module-based logging with multiple context keys
	logger.Info("\n--- Module-based logging demo ---")

	// Create module loggers with context keys
	authLogger := logger.With("module", "auth", "request_id", "req-123", "trace_id", "trace-001")
	dbLogger := logger.With("module", "database", "request_id", "req-123", "trace_id", "trace-002")
	apiLogger := logger.With("module", "api", "request_id", "req-456", "trace_id", "trace-003")

	// Demonstrate the output format: time [LEVEL] context module message │ attrs
	authLogger.Info("User login attempt", "user_id", "user-789", "method", "oauth2")
	dbLogger.Info("Query executed", "query", "SELECT * FROM users", "duration_ms", 12)
	apiLogger.Error("Request failed", "error", "timeout", "endpoint", "/users")

	// Show how it looks without module
	plainLogger := logger.With("request_id", "req-789")
	plainLogger.Info("Plain log message", "status", "ok")
}
