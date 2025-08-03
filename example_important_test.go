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
		"user_id", "12345", // This will be highlighted in yellow
		"username", "john.doe", // This will be normal color
		"request_id", "req-789", // This will be highlighted in yellow
	)

	logger.Error("Failed to process request",
		"error", "database timeout", // This will be highlighted in yellow
		"duration", "5.2s", // This will be normal color
		"user_id", "12345", // This will be highlighted in yellow
	)
}
