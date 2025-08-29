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
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
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
		logger.enabled = strings.ToLower(debugMode) == "true" || debugMode == "1"
	}

	if logLevel := os.Getenv("FEED_MCP_LOG_LEVEL"); logLevel != "" {
		logger.SetLevel(parseLogLevel(logLevel))
	}

	if jsonMode := os.Getenv("FEED_MCP_JSON_LOGS"); jsonMode != "" {
		logger.jsonMode = strings.ToLower(jsonMode) == "true" || jsonMode == "1"
	}

	return logger
}

// SetLevel sets the logging level
func (d *DebugLogger) SetLevel(level LogLevel) {
	d.level = level
}

// SetEnabled enables or disables debug logging
func (d *DebugLogger) SetEnabled(enabled bool) {
	d.enabled = enabled
}

// SetJSONMode enables or disables JSON formatted logs
func (d *DebugLogger) SetJSONMode(jsonMode bool) {
	d.jsonMode = jsonMode
}

// IsEnabled returns whether debug logging is enabled
func (d *DebugLogger) IsEnabled() bool {
	return d.enabled
}

// ShouldLog returns whether a message at the given level should be logged
func (d *DebugLogger) ShouldLog(level LogLevel) bool {
	return d.enabled && level <= d.level
}

// LogMessage represents a structured log message
type LogMessage struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	Operation string                 `json:"operation,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  string                 `json:"duration,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// log writes a log message at the specified level
func (d *DebugLogger) log(level LogLevel, message, component, operation, url string, err error, extra map[string]interface{}) {
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
		d.logJSON(logMsg)
	} else {
		d.logText(logMsg)
	}
}

// logJSON outputs the log message in JSON format
func (d *DebugLogger) logJSON(msg LogMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		// Fallback to simple text logging if JSON marshaling fails
		d.logger.Printf("ERROR: Failed to marshal log message to JSON: %v", err)
		return
	}
	d.logger.Println(string(data))
}

// logText outputs the log message in human-readable text format
func (d *DebugLogger) logText(msg LogMessage) {
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

// Debug logs a debug-level message
func (d *DebugLogger) Debug(message string) {
	d.log(LogLevelDebug, message, "", "", "", nil, nil)
}

// DebugWithContext logs a debug-level message with context
func (d *DebugLogger) DebugWithContext(message, component, operation, url string, extra map[string]interface{}) {
	d.log(LogLevelDebug, message, component, operation, url, nil, extra)
}

// Info logs an info-level message
func (d *DebugLogger) Info(message string) {
	d.log(LogLevelInfo, message, "", "", "", nil, nil)
}

// InfoWithContext logs an info-level message with context
func (d *DebugLogger) InfoWithContext(message, component, operation, url string, extra map[string]interface{}) {
	d.log(LogLevelInfo, message, component, operation, url, nil, extra)
}

// Warn logs a warning-level message
func (d *DebugLogger) Warn(message string) {
	d.log(LogLevelWarn, message, "", "", "", nil, nil)
}

// WarnWithContext logs a warning-level message with context
func (d *DebugLogger) WarnWithContext(message, component, operation, url string, err error, extra map[string]interface{}) {
	d.log(LogLevelWarn, message, component, operation, url, err, extra)
}

// Error logs an error-level message
func (d *DebugLogger) Error(message string, err error) {
	d.log(LogLevelError, message, "", "", "", err, nil)
}

// ErrorWithContext logs an error-level message with context
func (d *DebugLogger) ErrorWithContext(message, component, operation, url string, err error, extra map[string]interface{}) {
	d.log(LogLevelError, message, component, operation, url, err, extra)
}

// LogFeedError logs a FeedError with full context
func (d *DebugLogger) LogFeedError(feedErr *FeedError) {
	if feedErr == nil {
		return
	}

	extra := make(map[string]interface{})
	extra["error_id"] = feedErr.ID
	extra["error_type"] = feedErr.ErrorType
	extra["suggestion"] = feedErr.Suggestion

	if feedErr.HTTPStatus != 0 {
		extra["http_status"] = feedErr.HTTPStatus
	}

	if feedErr.HTTPHeaders != nil && len(feedErr.HTTPHeaders) > 0 {
		extra["http_headers"] = feedErr.HTTPHeaders
	}

	if feedErr.Attempt != 0 {
		extra["retry_attempt"] = feedErr.Attempt
		extra["max_attempts"] = feedErr.MaxAttempts
	}

	if feedErr.ParseContext != nil {
		extra["parse_line"] = feedErr.ParseContext.LineNumber
		extra["feed_format"] = feedErr.ParseContext.FeedFormat
	}

	d.log(LogLevelError, feedErr.Message, feedErr.Component, feedErr.Operation, feedErr.URL, feedErr.Cause, extra)
}

// Package-level convenience functions using the default logger

// SetDebugMode enables or disables debug mode for the default logger
func SetDebugMode(enabled bool) {
	defaultLogger.SetEnabled(enabled)
}

// SetLogLevel sets the log level for the default logger
func SetLogLevel(level LogLevel) {
	defaultLogger.SetLevel(level)
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	return defaultLogger.IsEnabled()
}

// DebugLog logs a debug message if debug mode is enabled
func DebugLog(message string) {
	defaultLogger.Debug(message)
}

// DebugLogWithContext logs a debug message with context
func DebugLogWithContext(message, component, operation, url string, extra map[string]interface{}) {
	defaultLogger.DebugWithContext(message, component, operation, url, extra)
}

// InfoLog logs an info message
func InfoLog(message string) {
	defaultLogger.Info(message)
}

// InfoLogWithContext logs an info message with context
func InfoLogWithContext(message, component, operation, url string, extra map[string]interface{}) {
	defaultLogger.InfoWithContext(message, component, operation, url, extra)
}

// WarnLog logs a warning message
func WarnLog(message string, err error) {
	defaultLogger.WarnWithContext(message, "", "", "", err, nil)
}

// ErrorLog logs an error message
func ErrorLog(message string, err error) {
	defaultLogger.Error(message, err)
}

// LogFeedError logs a FeedError using the default logger
func LogFeedError(feedErr *FeedError) {
	defaultLogger.LogFeedError(feedErr)
}

// Helper function to parse log level from string
func parseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "ERROR":
		return LogLevelError
	case "WARN", "WARNING":
		return LogLevelWarn
	case "INFO":
		return LogLevelInfo
	case "DEBUG":
		return LogLevelDebug
	default:
		return LogLevelInfo
	}
}
