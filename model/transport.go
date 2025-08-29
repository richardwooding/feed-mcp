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
	HTTPWithSSETransport
)

// ParseTransport converts a string to a Transport type
func ParseTransport(transport string) (Transport, error) {
	switch transport {
	case "stdio":
		return StdioTransport, nil
	case "http-with-sse":
		return HTTPWithSSETransport, nil
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
		return "http-with-sse"
	default:
		return "undefined"
	}
}
