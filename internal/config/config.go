// Package config persists MikroTik connection profiles so credentials can be
// entered once (e.g. via the TUI) and referenced by name from the MCP tools,
// instead of flowing through the LLM on every call.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is a saved RouterOS connection target.
type Profile struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	Port           int    `json:"port,omitempty"`
	User           string `json:"user"`
	Password       string `json:"password,omitempty"`
	UseTLS         bool   `json:"use_tls,omitempty"`
	TLSSkipVerify  bool   `json:"tls_skip_verify,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// Config is the on-disk document: a set of named profiles.
type Config struct {
	Profiles []Profile `json:"profiles"`
}

// envPath overrides the default config file location when set.
const envPath = "MIKROTIK_MCP_CONFIG"

// Path returns the resolved config file path. It honours MIKROTIK_MCP_CONFIG
// and otherwise falls back to <os.UserConfigDir>/mikrotik-mcp/profiles.json.
func Path() (string, error) {
	if p := strings.TrimSpace(os.Getenv(envPath)); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "mikrotik-mcp", "profiles.json"), nil
}

// Load reads the config file. A missing file yields an empty Config (no error).
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(p)
}

// LoadFrom reads the config from an explicit path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if len(data) > 0 {
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return &c, nil
}

// Save writes the config to the default path with 0600 permissions.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(p)
}

// SaveTo writes the config to an explicit path, creating parent directories
// (0700) and writing the file with 0600 permissions since it holds secrets.
func (c *Config) SaveTo(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}
	c.sort()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	// Write to a temp file then rename for atomicity, keeping 0600 perms.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func (c *Config) sort() {
	sort.SliceStable(c.Profiles, func(i, j int) bool {
		return strings.ToLower(c.Profiles[i].Name) < strings.ToLower(c.Profiles[j].Name)
	})
}

// Get returns the profile with the given name (case-insensitive) and whether
// it was found.
func (c *Config) Get(name string) (Profile, bool) {
	for _, p := range c.Profiles {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Profile{}, false
}

// Upsert inserts or replaces a profile by name (case-insensitive match).
func (c *Config) Upsert(p Profile) {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].Name, p.Name) {
			c.Profiles[i] = p
			return
		}
	}
	c.Profiles = append(c.Profiles, p)
}

// Remove deletes a profile by name. Returns true if a profile was removed.
func (c *Config) Remove(name string) bool {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].Name, name) {
			c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
			return true
		}
	}
	return false
}

// Validate checks required fields on a profile.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile name is required")
	}
	if strings.TrimSpace(p.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if strings.TrimSpace(p.User) == "" {
		return fmt.Errorf("user is required")
	}
	if p.Port < 0 || p.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	return nil
}
