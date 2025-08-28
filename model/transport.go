package model

import (
	"errors"
)

var ErrInvalidTransport = errors.New("invalid transport")

// Transport represents the communication transport method for the MCP server
type Transport uint8

const (
	UndefinedTransport Transport = iota
	StdioTransport
	HttpWithSSETransport
)

// ParseTransport converts a string to a Transport type
func ParseTransport(transport string) (Transport, error) {
	switch transport {
	case "stdio":
		return StdioTransport, nil
	case "http-with-sse":
		return HttpWithSSETransport, nil
	default:
		return UndefinedTransport, ErrInvalidTransport
	}
}

// String returns the string representation of a Transport
func (t Transport) String() string {
	switch t {
	case StdioTransport:
		return "stdio"
	case HttpWithSSETransport:
		return "http-with-sse"
	default:
		return "undefined"
	}
}
