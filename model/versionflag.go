package model

import (
	"fmt"

	"github.com/alecthomas/kong"
)

// VersionFlag is a custom flag type for displaying version information.
type VersionFlag string

// Decode implements the kong.DecodeContext interface.
func (v VersionFlag) Decode(ctx *kong.DecodeContext) error { return nil }

// IsBool implements the kong.BoolMapper interface.
func (v VersionFlag) IsBool() bool { return true }

// BeforeApply implements the kong.BeforeApply interface to handle version display.
func (v VersionFlag) BeforeApply(app *kong.Kong, vars kong.Vars) error {
	fmt.Println(vars["version"])
	app.Exit(0)
	return nil
}
