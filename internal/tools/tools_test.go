package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
)

func resultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content is not text: %T", res.Content[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("result is not JSON: %v (%s)", err, tc.Text)
	}
	return m
}

func callMultiCommand(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	h := &handlers{}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := h.multiCommand(context.Background(), req)
	if err != nil {
		t.Fatalf("multiCommand error: %v", err)
	}
	return res
}

func TestMultiCommandAggregatesPerDevice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	t.Setenv("MIKROTIK_MCP_CONFIG", path)

	c := &config.Config{}
	// 127.0.0.1:1 is closed -> dial fails fast; "ghost" is not saved.
	c.Upsert(config.Profile{Name: "down", Host: "127.0.0.1", User: "admin", Port: 1, TimeoutSeconds: 1})
	if err := c.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	out := resultJSON(t, callMultiCommand(t, map[string]any{
		"profiles":        []any{"down", "ghost"},
		"command":         "/system/resource/print",
		"timeout_seconds": float64(1),
	}))

	if out["targets"].(float64) != 2 {
		t.Fatalf("targets = %v, want 2", out["targets"])
	}
	if out["succeeded"].(float64) != 0 || out["failed"].(float64) != 2 {
		t.Fatalf("expected 0 succeeded / 2 failed, got %v / %v", out["succeeded"], out["failed"])
	}
	results := out["results"].([]any)
	byName := map[string]map[string]any{}
	for _, r := range results {
		m := r.(map[string]any)
		byName[m["profile"].(string)] = m
	}
	if byName["ghost"]["error"] != "unknown profile" {
		t.Fatalf("ghost error = %v, want 'unknown profile'", byName["ghost"]["error"])
	}
	if byName["down"]["error"] == "" || byName["down"]["error"] == nil {
		t.Fatal("expected a dial error for 'down'")
	}
}

func TestMultiCommandBlockedPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MIKROTIK_MCP_CONFIG", filepath.Join(dir, "profiles.json"))
	res := callMultiCommand(t, map[string]any{
		"profiles": []any{"x"},
		"command":  "/system/reboot",
	})
	if !res.IsError {
		t.Fatal("expected blocked-command policy error")
	}
}

func TestMultiCommandWildcardExpands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	t.Setenv("MIKROTIK_MCP_CONFIG", path)
	c := &config.Config{}
	c.Upsert(config.Profile{Name: "a", Host: "127.0.0.1", User: "u", Port: 1, TimeoutSeconds: 1})
	c.Upsert(config.Profile{Name: "b", Host: "127.0.0.1", User: "u", Port: 1, TimeoutSeconds: 1})
	if err := c.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	out := resultJSON(t, callMultiCommand(t, map[string]any{
		"profiles":        []any{"*"},
		"command":         "/system/resource/print",
		"timeout_seconds": float64(1),
	}))
	if out["targets"].(float64) != 2 {
		t.Fatalf("wildcard should target both profiles, got %v", out["targets"])
	}
}

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
