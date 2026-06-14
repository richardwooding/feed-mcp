// Package model provides debugging and logging utilities for enhanced error context.
package model

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents different logging levels
type LogLevel int

const (
	// LogLevelError represents the error logging level
	LogLevelError LogLevel = iota
	// LogLevelWarn represents the warning logging level
	LogLevelWarn
	// LogLevelInfo represents the info logging level
	LogLevelInfo
	// LogLevelDebug represents the debug logging level
	LogLevelDebug
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelError:
		return "ERROR"
	case LogLevelWarn:
		return "WARN"
	case LogLevelInfo:
		return "INFO"
	case LogLevelDebug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

// DebugLogger provides enhanced logging capabilities for debugging
type DebugLogger struct {
	level    LogLevel
	logger   *log.Logger
	enabled  bool
	jsonMode bool
}

// defaultLogger is the global logger instance
var defaultLogger *DebugLogger

func init() {
	defaultLogger = NewDebugLogger()
}

// NewDebugLogger creates a new debug logger with configuration from environment variables
func NewDebugLogger() *DebugLogger {
	logger := &DebugLogger{
		level:    LogLevelInfo,
		logger:   log.New(os.Stderr, "", 0), // No default prefix, we'll add our own
		enabled:  false,
		jsonMode: false,
	}

	// Configure from environment variables
	if debugMode := os.Getenv("FEED_MCP_DEBUG"); debugMode != "" {
		logger.enabled = strings.EqualFold(debugMode, "true") || debugMode == "1"
	}

	if logLevel := os.Getenv("FEED_MCP_LOG_LEVEL"); logLevel != "" {
		if level, err := parseLogLevel(logLevel); err != nil {
			// Log warning about invalid level but continue with default
			log.Printf("WARN: %v", err)
			logger.SetLevel(LogLevelInfo)
		} else {
			logger.SetLevel(level)
		}
	}

	if jsonMode := os.Getenv("FEED_MCP_JSON_LOGS"); jsonMode != "" {
		logger.jsonMode = strings.EqualFold(jsonMode, "true") || jsonMode == "1"
	}

	return logger
}

// SetLevel sets the logging level
func (d *DebugLogger) SetLevel(level LogLevel) {
	d.level = level
}

// ShouldLog returns whether a message at the given level should be logged
func (d *DebugLogger) ShouldLog(level LogLevel) bool {
	return d.enabled && level <= d.level
}

// LogMessage represents a structured log message
type LogMessage struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Component string         `json:"component,omitempty"`
	Operation string         `json:"operation,omitempty"`
	URL       string         `json:"url,omitempty"`
	Error     string         `json:"error,omitempty"`
	Duration  string         `json:"duration,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// log writes a log message at the specified level
func (d *DebugLogger) log(level LogLevel, message, component, operation, url string, err error, extra map[string]any) {
	if !d.ShouldLog(level) {
		return
	}

	logMsg := LogMessage{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		Component: component,
		Operation: operation,
		URL:       url,
		Extra:     extra,
	}

	if err != nil {
		logMsg.Error = err.Error()
	}

	if d.jsonMode {
		d.logJSON(&logMsg)
	} else {
		d.logText(&logMsg)
	}
}

// logJSON outputs the log message in JSON format
func (d *DebugLogger) logJSON(msg *LogMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		// Fallback to simple text logging if JSON marshaling fails
		d.logger.Printf("ERROR: Failed to marshal log message to JSON: %v", err)
		return
	}
	d.logger.Println(string(data))
}

// logText outputs the log message in human-readable text format
func (d *DebugLogger) logText(msg *LogMessage) {
	// Build the log message parts
	parts := []string{
		msg.Timestamp.Format("2006-01-02T15:04:05.000Z"),
		fmt.Sprintf("[%s]", msg.Level),
		msg.Message,
	}

	if msg.Component != "" {
		parts = append(parts, fmt.Sprintf("component=%s", msg.Component))
	}

	if msg.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation=%s", msg.Operation))
	}

	if msg.URL != "" {
		parts = append(parts, fmt.Sprintf("url=%s", msg.URL))
	}

	if msg.Error != "" {
		parts = append(parts, fmt.Sprintf("error=%q", msg.Error))
	}

	if msg.Duration != "" {
		parts = append(parts, fmt.Sprintf("duration=%s", msg.Duration))
	}

	// Add extra fields
	for key, value := range msg.Extra {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	d.logger.Println(strings.Join(parts, " "))
}

// DebugWithContext logs a debug-level message with context
func (d *DebugLogger) DebugWithContext(message, component, operation, url string, extra map[string]any) {
	d.log(LogLevelDebug, message, component, operation, url, nil, extra)
}

// DebugLogWithContext logs a debug message with context using the default logger
func DebugLogWithContext(message, component, operation, url string, extra map[string]any) {
	defaultLogger.DebugWithContext(message, component, operation, url, extra)
}

// Helper function to parse log level from string
func parseLogLevel(level string) (LogLevel, error) {
	switch strings.ToUpper(level) {
	case "ERROR":
		return LogLevelError, nil
	case "WARN", "WARNING":
		return LogLevelWarn, nil
	case "INFO":
		return LogLevelInfo, nil
	case "DEBUG":
		return LogLevelDebug, nil
	default:
		return LogLevelInfo, fmt.Errorf("invalid log level: %s, defaulting to INFO", level)
	}
}
