package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func typeStr(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// Drives the model to add a new profile and confirms it is persisted.
func TestAddProfileFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	m := tea.Model(newModel(&config.Config{}, path))

	// 'a' opens the add form (Name focused).
	m = send(m, key("a"))
	m = typeStr(m, "lab")
	m = send(m, key("tab")) // -> Host
	m = typeStr(m, "192.168.88.1")
	m = send(m, key("tab")) // -> Port
	m = typeStr(m, "8728")
	m = send(m, key("tab")) // -> User
	m = typeStr(m, "admin")
	m = send(m, key("tab")) // -> Password
	m = typeStr(m, "pw")
	m = send(m, key("tab"))   // -> Use TLS
	m = send(m, key("space")) // toggle TLS on
	m = send(m, key("ctrl+s"))

	mm := m.(model)
	if mm.errMsg != "" {
		t.Fatalf("unexpected form error: %s", mm.errMsg)
	}
	if mm.screen != screenList {
		t.Fatalf("expected to return to list, got screen %d", mm.screen)
	}

	saved, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := saved.Get("lab")
	if !ok {
		t.Fatal("profile not saved")
	}
	if p.Host != "192.168.88.1" || p.User != "admin" || p.Password != "pw" || p.Port != 8728 || !p.UseTLS {
		t.Fatalf("saved profile mismatch: %+v", p)
	}
}

func TestAddProfileValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	m := tea.Model(newModel(&config.Config{}, path))

	// Open form, leave name empty, try to save.
	m = send(m, key("a"), key("ctrl+s"))
	mm := m.(model)
	if mm.errMsg == "" {
		t.Fatal("expected validation error for empty profile")
	}
	if mm.screen != screenForm {
		t.Fatal("should stay on form when invalid")
	}
}

func TestDeleteProfileFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	c := &config.Config{}
	c.Upsert(config.Profile{Name: "gone", Host: "h", User: "u"})
	if err := c.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	m := tea.Model(newModel(c, path))
	m = send(m, key("d"), key("y")) // delete, confirm yes
	mm := m.(model)
	if !strings.Contains(mm.status, "Deleted") {
		t.Fatalf("expected deletion status, got %q", mm.status)
	}
	saved, _ := config.LoadFrom(path)
	if _, ok := saved.Get("gone"); ok {
		t.Fatal("profile should have been removed from disk")
	}
}
