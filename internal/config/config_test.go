package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "profiles.json")

	c := &Config{}
	c.Upsert(Profile{Name: "rtr", Host: "10.0.0.1", User: "admin", Password: "secret", UseTLS: true, Port: 8729})
	if err := c.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// File must be 0600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}

	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p, ok := got.Get("RTR") // case-insensitive
	if !ok {
		t.Fatal("profile not found")
	}
	if p.Host != "10.0.0.1" || p.User != "admin" || p.Password != "secret" || !p.UseTLS || p.Port != 8729 {
		t.Fatalf("round trip mismatch: %+v", p)
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, err := LoadFrom(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(c.Profiles) != 0 {
		t.Fatalf("expected empty config, got %d", len(c.Profiles))
	}
}

func TestUpsertReplacesAndRemove(t *testing.T) {
	c := &Config{}
	c.Upsert(Profile{Name: "a", Host: "h1", User: "u"})
	c.Upsert(Profile{Name: "A", Host: "h2", User: "u"}) // same name, different case
	if len(c.Profiles) != 1 {
		t.Fatalf("expected 1 profile after upsert, got %d", len(c.Profiles))
	}
	if p, _ := c.Get("a"); p.Host != "h2" {
		t.Fatalf("upsert did not replace: %+v", p)
	}
	if !c.Remove("a") {
		t.Fatal("remove returned false")
	}
	if len(c.Profiles) != 0 {
		t.Fatalf("expected 0 profiles after remove, got %d", len(c.Profiles))
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		p    Profile
		ok   bool
	}{
		{"valid", Profile{Name: "x", Host: "h", User: "u"}, true},
		{"no name", Profile{Host: "h", User: "u"}, false},
		{"no host", Profile{Name: "x", User: "u"}, false},
		{"no user", Profile{Name: "x", Host: "h"}, false},
		{"bad port", Profile{Name: "x", Host: "h", User: "u", Port: 70000}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestPathHonoursEnv(t *testing.T) {
	t.Setenv(envPath, "/tmp/custom/profiles.json")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/tmp/custom/profiles.json" {
		t.Fatalf("env override ignored: %s", p)
	}
}
