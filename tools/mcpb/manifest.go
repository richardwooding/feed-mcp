package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Manifest is the subset of the MCP Bundle manifest.json (manifest_version
// "0.3") that this packer emits. See https://github.com/anthropics/mcpb.
type Manifest struct {
	ManifestVersion string                     `json:"manifest_version"`
	Name            string                     `json:"name"`
	DisplayName     string                     `json:"display_name,omitempty"`
	Version         string                     `json:"version"`
	Description     string                     `json:"description"`
	Author          Author                     `json:"author"`
	Server          Server                     `json:"server"`
	UserConfig      map[string]UserConfigField `json:"user_config,omitempty"`
	Compatibility   *Compatibility             `json:"compatibility,omitempty"`
}

type Author struct {
	Name string `json:"name"`
}

type Server struct {
	Type              string                      `json:"type"`
	EntryPoint        string                      `json:"entry_point"`
	MCPConfig         MCPConfig                   `json:"mcp_config"`
	PlatformOverrides map[string]PlatformOverride `json:"platform_overrides,omitempty"`
}

type MCPConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type PlatformOverride struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type UserConfigField struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Multiple    bool   `json:"multiple,omitempty"`
	Default     string `json:"default,omitempty"`
}

type Compatibility struct {
	Platforms []string `json:"platforms,omitempty"`
}

// validate checks the fields the MCP Bundle spec marks as required. The Node
// CLI would do this against a JSON schema; here we fail fast on empties.
func (m Manifest) validate() error {
	missing := func(name, val string) error {
		if val == "" {
			return fmt.Errorf("manifest: %s is required", name)
		}
		return nil
	}
	for _, e := range []error{
		missing("manifest_version", m.ManifestVersion),
		missing("name", m.Name),
		missing("version", m.Version),
		missing("description", m.Description),
		missing("author.name", m.Author.Name),
		missing("server.type", m.Server.Type),
		missing("server.entry_point", m.Server.EntryPoint),
		missing("server.mcp_config.command", m.Server.MCPConfig.Command),
	} {
		if e != nil {
			return e
		}
	}
	return nil
}

func (m Manifest) writeJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}
