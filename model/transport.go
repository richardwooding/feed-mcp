package model

import (
	"errors"
)

// ErrInvalidTransport is returned when an invalid transport type is specified.
var ErrInvalidTransport = errors.New("invalid transport")

// Transport name strings used for parsing and string representation.
const (
	transportNameStdio = "stdio"
	// transportNameHTTPWithSSE is a transport identifier, not a credential.
	transportNameHTTPWithSSE    = "http-with-sse" //nolint:gosec // G101 false positive: transport name, not a secret
	transportNameStreamableHTTP = "streamable-http"
	transportNameUndefined      = "undefined"
)

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
	case transportNameStdio:
		return StdioTransport, nil
	case transportNameHTTPWithSSE:
		// Deprecated: maps to StreamableHTTPTransport for backwards compatibility
		return StreamableHTTPTransport, nil
	case transportNameStreamableHTTP:
		return StreamableHTTPTransport, nil
	default:
		return UndefinedTransport, ErrInvalidTransport
	}
}

// String returns the string representation of a Transport
func (t Transport) String() string {
	switch t {
	case StdioTransport:
		return transportNameStdio
	case HTTPWithSSETransport:
		return transportNameHTTPWithSSE // Deprecated
	case StreamableHTTPTransport:
		return transportNameStreamableHTTP
	default:
		return transportNameUndefined
	}
}
