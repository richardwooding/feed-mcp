package model

import (
	"testing"
)

func TestParseTransport(t *testing.T) {
	tests := []struct {
		input   string
		want    Transport
		wantErr bool
	}{
		{"stdio", StdioTransport, false},
		{"http-with-sse", HTTPWithSSETransport, false},
		{"invalid", UndefinedTransport, true},
		{"", UndefinedTransport, true},
	}
	for _, tt := range tests {
		got, err := ParseTransport(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseTransport(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("ParseTransport(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTransportString(t *testing.T) {
	tests := []struct {
		want string
		tr   Transport
	}{
		{"stdio", StdioTransport},
		{"http-with-sse", HTTPWithSSETransport},
		{"undefined", UndefinedTransport},
		{"undefined", Transport(99)},
	}
	for _, tt := range tests {
		if got := tt.tr.String(); got != tt.want {
			t.Errorf("Transport(%d).String() = %q, want %q", tt.tr, got, tt.want)
		}
	}
}
