package trifle

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"miren.dev/trifle/pkg/color"
)

func TestImportantKeys(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	tests := []struct {
		name          string
		importantKeys []string
		logFunc       func(logger *slog.Logger)
		expectedKeys  map[string]bool // map of keys that should be yellow
	}{
		{
			name:          "highlight important keys",
			importantKeys: []string{"user_id", "error"},
			logFunc: func(logger *slog.Logger) {
				logger.Info("test message",
					"user_id", "12345",
					"name", "John Doe",
					"error", "something went wrong",
				)
			},
			expectedKeys: map[string]bool{
				"user_id": true,
				"error":   true,
				"name":    false,
			},
		},
		{
			name:          "no important keys",
			importantKeys: nil,
			logFunc: func(logger *slog.Logger) {
				logger.Info("test message", "key1", "value1", "key2", "value2")
			},
			expectedKeys: map[string]bool{
				"key1": false,
				"key2": false,
			},
		},
		{
			name:          "empty important keys",
			importantKeys: []string{},
			logFunc: func(logger *slog.Logger) {
				logger.Info("test message", "key1", "value1")
			},
			expectedKeys: map[string]bool{
				"key1": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// Create handler with important keys
			handler := New(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}, WithImportantKeys(tt.importantKeys...))

			logger := slog.New(handler)

			// Execute the log function
			tt.logFunc(logger)

			output := buf.String()
			require.NotEmpty(t, output)

			// Check that important keys are highlighted with yellow color
			yellowColor := color.New(color.FgHiYellow)
			faintBoldColor := color.New(color.Faint, color.Bold)

			for key, shouldBeYellow := range tt.expectedKeys {
				if shouldBeYellow {
					// Check that the key is wrapped with yellow color codes
					expectedKey := yellowColor.Sprint(key)
					assert.Contains(t, output, expectedKey, "Key %s should be highlighted in yellow", key)
				} else {
					// Check that the key is wrapped with faint bold color codes
					expectedKey := faintBoldColor.Sprint(key)
					assert.Contains(t, output, expectedKey, "Key %s should not be highlighted in yellow", key)
				}
			}
		})
	}
}

func TestImportantKeysWithGroups(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	// Create handler with important keys
	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithImportantKeys("request_id", "user_id"))

	logger := slog.New(handler).With("request_id", "req-123")
	logger = logger.WithGroup("user")
	logger.Info("user action", "user_id", "user-456", "action", "login")

	output := buf.String()
	require.NotEmpty(t, output)

	// Check that important keys are highlighted even with groups
	yellowColor := color.New(color.FgHiYellow)
	assert.Contains(t, output, yellowColor.Sprint("request_id"), "request_id should be highlighted")
	assert.Contains(t, output, yellowColor.Sprint("user_id"), "user.user_id should be highlighted")
}

func TestImportantKeysCloning(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	// Create handler with important keys
	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithImportantKeys("important"))

	// Create a logger with attributes
	logger := slog.New(handler).With("normal", "value1")

	// Log with both normal and important keys
	logger.Info("test", "important", "highlighted")

	output := buf.String()
	require.NotEmpty(t, output)

	// Check that the important key is highlighted
	yellowColor := color.New(color.FgHiYellow)
	assert.Contains(t, output, yellowColor.Sprint("important"), "important key should be highlighted")

	// Check that normal key is not highlighted
	faintBoldColor := color.New(color.Faint, color.Bold)
	assert.Contains(t, output, faintBoldColor.Sprint("normal"), "normal key should not be highlighted")
}

func TestCriticalKeys(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	tests := []struct {
		name           string
		importantKeys  []string
		criticalKeys   []string
		logFunc        func(logger *slog.Logger)
		expectedColors map[string]*color.Color
	}{
		{
			name:          "critical keys in red",
			criticalKeys:  []string{"error", "panic"},
			importantKeys: []string{"user_id", "request_id"},
			logFunc: func(logger *slog.Logger) {
				logger.Error("System failure",
					"error", "database connection lost",
					"panic", "nil pointer",
					"user_id", "12345",
					"severity", "high",
				)
			},
			expectedColors: map[string]*color.Color{
				"error":    color.New(color.FgHiRed),           // Critical - red
				"panic":    color.New(color.FgHiRed),           // Critical - red
				"user_id":  color.New(color.FgHiYellow),        // Important - yellow
				"severity": color.New(color.Faint, color.Bold), // Normal - faint
			},
		},
		{
			name:          "priority: critical > important",
			criticalKeys:  []string{"error"},
			importantKeys: []string{"error", "user_id"}, // error is in both
			logFunc: func(logger *slog.Logger) {
				logger.Info("Test priority",
					"error", "test error",
					"user_id", "789",
				)
			},
			expectedColors: map[string]*color.Color{
				"error":   color.New(color.FgHiRed),    // Critical takes precedence
				"user_id": color.New(color.FgHiYellow), // Important
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			opts := []Option{}
			if len(tt.importantKeys) > 0 {
				opts = append(opts, WithImportantKeys(tt.importantKeys...))
			}
			if len(tt.criticalKeys) > 0 {
				opts = append(opts, WithCriticalKeys(tt.criticalKeys...))
			}

			handler := New(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}, opts...)

			logger := slog.New(handler)

			// Execute the log function
			tt.logFunc(logger)

			output := buf.String()
			require.NotEmpty(t, output)

			// Check that keys have the expected colors
			for key, expectedColor := range tt.expectedColors {
				coloredKey := expectedColor.Sprint(key)
				assert.Contains(t, output, coloredKey, "Key %s should have the expected color", key)
			}
		})
	}
}

func TestCriticalKeysWithGroups(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithCriticalKeys("error", "failure"))

	logger := slog.New(handler).WithGroup("service")
	logger.Error("Service error", "error", "timeout", "retry", 3)

	output := buf.String()
	require.NotEmpty(t, output)

	// Check that critical key is red even in groups
	redColor := color.New(color.FgHiRed)
	assert.Contains(t, output, redColor.Sprint("error"), "Critical key should be red even in groups")
}

func TestContextKey(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	tests := []struct {
		name           string
		contextKeys    []string
		logFunc        func(logger *slog.Logger)
		expectedOutput []string
		notExpected    []string
	}{
		{
			name:        "context key from record attrs",
			contextKeys: []string{"request_id"},
			logFunc: func(logger *slog.Logger) {
				logger.Info("Processing request", "request_id", "req-123", "method", "GET")
			},
			expectedOutput: []string{
				"req-123",            // Context value appears
				"Processing request", // Message appears
				"method",             // Other attrs still shown
				"GET",
			},
			notExpected: []string{
				"request_id: req-123", // Should not appear in regular attrs
			},
		},
		{
			name:        "context key from WithAttrs",
			contextKeys: []string{"request_id"},
			logFunc: func(logger *slog.Logger) {
				logger = logger.With("request_id", "req-456")
				logger.Info("User logged in", "user_id", "user-789")
			},
			expectedOutput: []string{
				"req-456",
				"User logged in",
				"user_id",
				"user-789",
			},
			notExpected: []string{
				"request_id: req-456", // Should not appear in regular attrs
			},
		},
		{
			name:        "no context key set",
			contextKeys: []string{},
			logFunc: func(logger *slog.Logger) {
				logger.Info("Normal log", "request_id", "req-999", "status", "ok")
			},
			expectedOutput: []string{
				"Normal log",
				"request_id",
				"req-999",
				"status",
				"ok",
			},
			notExpected: []string{
				"req-999 Normal log", // Should not have context prefix
			},
		},
		{
			name:        "context key not present",
			contextKeys: []string{"request_id"},
			logFunc: func(logger *slog.Logger) {
				logger.Info("No request ID", "user_id", "user-123")
			},
			expectedOutput: []string{
				"No request ID",
				"user_id",
				"user-123",
			},
			notExpected: []string{
				"user-123 No request ID", // Should not use wrong key as context
			},
		},
		{
			name:        "multiple context keys all present",
			contextKeys: []string{"request_id", "user_id", "session_id"},
			logFunc: func(logger *slog.Logger) {
				logger.Info("Multiple contexts",
					"request_id", "req-123",
					"user_id", "user-456",
					"session_id", "sess-789",
					"action", "login")
			},
			expectedOutput: []string{
				"req-123",  // First context
				"user-456", // Second context
				"sess-789", // Third context
				"Multiple contexts",
				"action",
				"login",
			},
			notExpected: []string{
				"request_id: req-123",
				"user_id: user-456",
				"session_id: sess-789",
			},
		},
		{
			name:        "multiple context keys some missing",
			contextKeys: []string{"request_id", "missing_key", "user_id"},
			logFunc: func(logger *slog.Logger) {
				logger.Info("Partial contexts",
					"request_id", "req-999",
					"user_id", "user-111",
					"status", "ok")
			},
			expectedOutput: []string{
				"req-999",  // First context present
				"user-111", // Third context present (missing_key skipped)
				"Partial contexts",
				"status",
				"ok",
			},
			notExpected: []string{
				"request_id: req-999",
				"user_id: user-111",
			},
		},
		{
			name:        "multiple context keys from WithAttrs",
			contextKeys: []string{"request_id", "user_id"},
			logFunc: func(logger *slog.Logger) {
				logger = logger.With("request_id", "req-abc", "user_id", "user-xyz")
				logger.Info("From attrs", "action", "update")
			},
			expectedOutput: []string{
				"req-abc",
				"user-xyz",
				"From attrs",
				"action",
				"update",
			},
			notExpected: []string{
				"request_id: req-abc",
				"user_id: user-xyz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			opts := []Option{}
			if len(tt.contextKeys) > 0 {
				opts = append(opts, WithContextKey(tt.contextKeys...))
			}

			handler := New(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}, opts...)

			logger := slog.New(handler)

			// Execute the log function
			tt.logFunc(logger)

			output := buf.String()
			require.NotEmpty(t, output)

			// Check expected output
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}

			// Check not expected
			for _, notExpected := range tt.notExpected {
				assert.NotContains(t, output, notExpected, "Output should not contain: %s", notExpected)
			}
		})
	}
}

func TestContextKeyWithGroups(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithContextKey("request_id", "user_id"))

	logger := slog.New(handler).With("request_id", "req-group", "user_id", "user-base")
	logger = logger.WithGroup("user")

	// Context key should still work even with groups
	logger.Info("User action", "id", "user-123", "action", "login")

	output := buf.String()
	require.NotEmpty(t, output)

	// Check that both context values appear before message
	assert.Contains(t, output, "req-group", "First context should appear")
	assert.Contains(t, output, "user-base", "Second context should appear")
	assert.Contains(t, output, "User action", "Message should appear")
	// Check that context keys don't appear in attrs
	assert.NotContains(t, output, "request_id:", "Context key should not appear in attributes")
	assert.NotContains(t, output, "user_id:", "Context key should not appear in attributes")
	// Check that grouped attrs still appear
	assert.Contains(t, output, "user.", "Grouped attributes should appear")
	assert.Contains(t, output, "id", "Grouped attribute key should appear")
	assert.Contains(t, output, "user-123", "Grouped attribute value should appear")
}

func TestMultipleContextKeysWithGroups(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithContextKey("request_id", "session_id", "trace_id"))

	// Add some context keys at different levels
	logger := slog.New(handler).With("request_id", "req-main")
	serviceLogger := logger.WithGroup("service").With("session_id", "sess-123")
	dbLogger := serviceLogger.WithGroup("db").With("trace_id", "trace-xyz")

	// Log with all three context keys
	dbLogger.Info("Query executed", "table", "users", "rows", 42)

	output := buf.String()
	require.NotEmpty(t, output)

	// Verify all three context values appear in order
	assert.Regexp(t, `req-main.*sess-123.*trace-xyz.*Query executed`, output, "All context values should appear in order before message")

	// Verify context keys don't appear in attributes
	assert.NotContains(t, output, "request_id:", "Context keys should not appear in attributes")
	assert.NotContains(t, output, "session_id:", "Context keys should not appear in attributes")
	assert.NotContains(t, output, "trace_id:", "Context keys should not appear in attributes")
}
