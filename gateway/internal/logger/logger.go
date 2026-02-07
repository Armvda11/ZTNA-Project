package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ztna/gateway/internal/config"
)

// Level represents log level
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides structured logging
type Logger struct {
	level  Level
	format string
	output io.Writer
}

// New creates a new logger from configuration
func New(cfg config.LoggingConfig) *Logger {
	level := parseLevel(cfg.Level)
	output := parseOutput(cfg.Output)

	return &Logger{
		level:  level,
		format: cfg.Format,
		output: output,
	}
}

func parseLevel(level string) Level {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

func parseOutput(output string) io.Writer {
	switch strings.ToLower(output) {
	case "stdout", "":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		// Try to open file
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("Failed to open log file %s: %v, using stdout", output, err)
			return os.Stdout
		}
		return f
	}
}

// log writes a log message
func (l *Logger) log(level Level, msg string, keysAndValues ...interface{}) {
	if level < l.level {
		return
	}

	if l.format == "json" {
		l.logJSON(level, msg, keysAndValues...)
	} else {
		l.logText(level, msg, keysAndValues...)
	}
}

func (l *Logger) logJSON(level Level, msg string, keysAndValues ...interface{}) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     level.String(),
		"message":   msg,
	}

	// Add key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprint(keysAndValues[i])
			entry[key] = keysAndValues[i+1]
		}
	}

	data, _ := json.Marshal(entry)
	fmt.Fprintf(l.output, "%s\n", data)
}

func (l *Logger) logText(level Level, msg string, keysAndValues ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	extra := ""
	if len(keysAndValues) > 0 {
		parts := make([]string, 0, len(keysAndValues)/2)
		for i := 0; i < len(keysAndValues); i += 2 {
			if i+1 < len(keysAndValues) {
				key := fmt.Sprint(keysAndValues[i])
				val := fmt.Sprint(keysAndValues[i+1])
				parts = append(parts, fmt.Sprintf("%s=%s", key, val))
			}
		}
		if len(parts) > 0 {
			extra = " " + strings.Join(parts, " ")
		}
	}

	fmt.Fprintf(l.output, "[%s] %-5s %s%s\n", timestamp, level.String(), msg, extra)
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.log(DebugLevel, msg, keysAndValues...)
}

// Info logs an info message
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	l.log(InfoLevel, msg, keysAndValues...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	l.log(WarnLevel, msg, keysAndValues...)
}

// Error logs an error message
func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	l.log(ErrorLevel, msg, keysAndValues...)
}
