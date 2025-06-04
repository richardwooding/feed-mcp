package main

import (
	"testing"

	"github.com/alecthomas/kong"
)

func TestCLI_Parse_RunCommand(t *testing.T) {
	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	_, err = parser.Parse([]string{"run", "--transport=stdio", "http://example.com"})
	if err != nil {
		t.Errorf("failed to parse run command: %v", err)
	}
}
