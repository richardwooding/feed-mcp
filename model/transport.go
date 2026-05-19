package model

import (
	"errors"
)

// ErrInvalidTransport is returned when an invalid transport type is specified.
var ErrInvalidTransport = errors.New("invalid transport")

// Transport represents the communication transport method for the MCP server
type Transport uint8

// Transport constants define the available transport types.
const (
	UndefinedTransport Transport = iota
	StdioTransport
	HTTPWithSSETransport    // Deprecated: use StreamableHTTPTransport instead
	StreamableHTTPTransport // Streamable HTTP transport per MCP spec
)

// ParseTransport converts a string to a Transport type
func ParseTransport(transport string) (Transport, error) {
	switch transport {
	case "stdio":
		return StdioTransport, nil
	case "http-with-sse":
		// Deprecated: maps to StreamableHTTPTransport for backwards compatibility
		return StreamableHTTPTransport, nil
	case "streamable-http":
		return StreamableHTTPTransport, nil
	default:
		return UndefinedTransport, ErrInvalidTransport
	}
}

// String returns the string representation of a Transport
func (t Transport) String() string {
	switch t {
	case StdioTransport:
		return "stdio"
	case HTTPWithSSETransport:
		return "http-with-sse" // Deprecated
	case StreamableHTTPTransport:
		return "streamable-http"
	default:
		return "undefined"
	}
}
