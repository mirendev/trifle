package trifle

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

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
					// Also check that the value is underlined
					// This is a simple check - in reality values might be quoted or formatted differently
					assert.Contains(t, output, "\x1b[4m", "Values for important keys should be underlined")
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

func TestImportantValuesUnderlined(t *testing.T) {
	// Disable color detection for consistent testing
	color.NoColor = false

	var buf bytes.Buffer

	// Create handler with important keys
	handler := New(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}, WithImportantKeys("str_key", "int_key", "bool_key", "time_key"))

	logger := slog.New(handler)

	// Test various value types
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	logger.Info("test values",
		"str_key", "hello world", // Should be underlined
		"int_key", 42, // Should be underlined
		"bool_key", true, // Should be underlined
		"time_key", testTime, // Should be underlined
		"normal_str", "not important", // Should NOT be underlined
		"normal_int", 99, // Should NOT be underlined
	)

	output := buf.String()
	require.NotEmpty(t, output)

	// Check that important values are underlined
	underlineColor := color.New(color.Underline)

	// Check string value (quoted)
	assert.Contains(t, output, underlineColor.Sprint("\"hello world\""), "String value should be underlined")
	// Check int value
	assert.Contains(t, output, underlineColor.Sprint("42"), "Int value should be underlined")
	// Check bool value
	assert.Contains(t, output, underlineColor.Sprint("true"), "Bool value should be underlined")

	// Check that normal values are NOT underlined
	assert.NotContains(t, output, underlineColor.Sprint("\"not important\""), "Normal string should not be underlined")
	assert.NotContains(t, output, underlineColor.Sprint("99"), "Normal int should not be underlined")
}
