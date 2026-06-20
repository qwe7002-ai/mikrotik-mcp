package tools

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
)

func TestTargetInlineOnly(t *testing.T) {
	tgt, err := target(map[string]any{
		"host":            "1.2.3.4",
		"user":            "admin",
		"password":        "p",
		"port":            float64(8729),
		"use_tls":         true,
		"timeout_seconds": float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Host != "1.2.3.4" || tgt.User != "admin" || tgt.Password != "p" {
		t.Fatalf("unexpected target: %+v", tgt)
	}
	if tgt.Port != 8729 || !tgt.UseTLS || tgt.Timeout != 5*time.Second {
		t.Fatalf("unexpected target options: %+v", tgt)
	}
}

func TestTargetMissingHost(t *testing.T) {
	if _, err := target(map[string]any{"user": "admin"}); err == nil {
		t.Fatal("expected missing host error")
	}
}

func TestTargetFromProfileWithOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	t.Setenv("MIKROTIK_MCP_CONFIG", path)

	c := &config.Config{}
	c.Upsert(config.Profile{
		Name: "home", Host: "10.0.0.1", User: "admin", Password: "secret",
		UseTLS: true, Port: 8729, TimeoutSeconds: 12,
	})
	if err := c.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	// Profile supplies everything; inline host overrides.
	tgt, err := target(map[string]any{
		"profile": "home",
		"host":    "10.0.0.99",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Host != "10.0.0.99" {
		t.Fatalf("inline host should override profile, got %s", tgt.Host)
	}
	if tgt.User != "admin" || tgt.Password != "secret" || !tgt.UseTLS || tgt.Port != 8729 {
		t.Fatalf("profile fields not applied: %+v", tgt)
	}
	if tgt.Timeout != 12*time.Second {
		t.Fatalf("profile timeout not applied: %v", tgt.Timeout)
	}
}

func TestTargetUnknownProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MIKROTIK_MCP_CONFIG", filepath.Join(dir, "profiles.json"))
	if _, err := target(map[string]any{"profile": "nope"}); err == nil {
		t.Fatal("expected unknown profile error")
	}
}
