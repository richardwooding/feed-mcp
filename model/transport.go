package model

import (
	"errors"
)

var ErrInvalidTransport = errors.New("invalid transport")

type Transport uint8

const (
	UndefinedTransport Transport = iota
	StdioTransport
	HttpWithSSETransport
)

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
