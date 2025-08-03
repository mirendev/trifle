package trifle_test

import (
	"log/slog"
	"os"

	"miren.dev/trifle"
)

func ExampleWithImportantKeys() {
	// Create a logger with important keys highlighted in yellow
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, trifle.WithImportantKeys("user_id", "error", "request_id"))

	logger := slog.New(handler)

	// Log messages with various keys
	logger.Info("User login attempt",
		"user_id", "12345", // Key will be highlighted in yellow
		"username", "john.doe", // Key will be normal color
		"request_id", "req-789", // Key will be highlighted in yellow
	)

	logger.Error("Failed to process request",
		"error", "database timeout", // Key will be highlighted in yellow
		"duration", "5.2s", // Key will be normal color
		"user_id", "12345", // Key will be highlighted in yellow
	)
}

func ExampleWithContextKey() {
	// Create a logger with a context key that appears before each message
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, trifle.WithContextKey("request_id"))

	logger := slog.New(handler)

	// Add request_id that will appear before all messages
	requestLogger := logger.With("request_id", "req-123")

	// Log messages - notice request_id appears before the message
	requestLogger.Info("Processing user login", "user_id", "user-456")
	requestLogger.Info("Validating credentials", "method", "oauth2")
	requestLogger.Error("Login failed", "error", "invalid token")

	// request_id won't appear in the regular attributes section
}

func ExampleWithContextKey_multiple() {
	// Create a logger with multiple context keys
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, trifle.WithContextKey("request_id", "user_id", "session_id"))

	logger := slog.New(handler)

	// Add context values
	logger = logger.With("request_id", "req-789", "session_id", "sess-xyz")

	// Log messages - context values appear in order before message
	logger.Info("User authenticated", "user_id", "user-123", "method", "oauth2")
	logger.Info("Profile updated", "fields", []string{"email", "name"})

	// If a context key is missing, it's skipped
	logger.Info("Session expired") // Only req-789 and sess-xyz appear
}

func ExampleWithCriticalKeys() {
	// Create a logger with critical keys highlighted in red
	handler := trifle.New(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	},
		trifle.WithCriticalKeys("error", "panic", "failure"),
		trifle.WithImportantKeys("user_id", "request_id"),
	)

	logger := slog.New(handler)

	// Log messages with different key priorities
	logger.Error("Database connection failed",
		"error", "connection timeout", // Critical - shown in red
		"user_id", "user-123", // Important - shown in yellow
		"retry_count", 3, // Normal - shown in default color
	)

	logger.Error("Critical system failure",
		"panic", "nil pointer dereference", // Critical - shown in red
		"stack_trace", "...", // Normal - shown in default color
	)
}
