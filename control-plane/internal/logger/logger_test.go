package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ztna/control-plane/internal/config"
)

func TestLogger(t *testing.T) {
	tests := []struct {
		name   string
		level  string
		format string
		logFn  func(*Logger)
		want   string
	}{
		{
			name:   "info json",
			level:  "info",
			format: "json",
			logFn: func(l *Logger) {
				l.Info("test message", "key", "value")
			},
			want: `"message":"test message"`,
		},
		{
			name:   "error text",
			level:  "error",
			format: "text",
			logFn: func(l *Logger) {
				l.Error("error occurred", "code", 500)
			},
			want: "ERROR error occurred",
		},
		{
			name:   "debug below level",
			level:  "info",
			format: "text",
			logFn: func(l *Logger) {
				l.Debug("debug message")
			},
			want: "", // Should not log
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := &Logger{
				level:  parseLevel(tt.level),
				format: tt.format,
				output: &buf,
			}

			tt.logFn(logger)

			output := buf.String()
			if tt.want == "" && output != "" {
				t.Errorf("Expected no output, got: %s", output)
			}
			if tt.want != "" && !strings.Contains(output, tt.want) {
				t.Errorf("Expected output to contain '%s', got: %s", tt.want, output)
			}
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", DebugLevel},
		{"DEBUG", DebugLevel},
		{"info", InfoLevel},
		{"warn", WarnLevel},
		{"warning", WarnLevel},
		{"error", ErrorLevel},
		{"invalid", InfoLevel}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}

	logger := New(cfg)
	if logger == nil {
		t.Fatal("Expected logger, got nil")
	}

	if logger.level != DebugLevel {
		t.Errorf("Expected DebugLevel, got %v", logger.level)
	}

	if logger.format != "json" {
		t.Errorf("Expected json format, got %s", logger.format)
	}
}
