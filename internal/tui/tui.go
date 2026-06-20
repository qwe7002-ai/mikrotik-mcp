// Package tui provides an interactive terminal UI for managing saved MikroTik
// connection profiles, so users can add login information from the CLI without
// hand-editing the JSON config file.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qwe7002/mikrotik-mcp/internal/config"
)

// Run launches the profile-management TUI and blocks until the user quits.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	m := newModel(cfg, path)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

type screen int

const (
	screenList screen = iota
	screenForm
	screenConfirmDelete
)

// form field indexes
const (
	fName = iota
	fHost
	fPort
	fUser
	fPassword
	fUseTLS
	fTLSSkipVerify
	fTimeout
	fieldCount
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	focusedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

type model struct {
	cfg  *config.Config
	path string

	screen screen
	cursor int // selection in list
	status string
	errMsg string

	// form state
	inputs    []textinput.Model
	bools     [fieldCount]bool // only fUseTLS / fTLSSkipVerify used
	focus     int
	editingOf string // name of profile being edited; "" means new
	quitting  bool
}

func newModel(cfg *config.Config, path string) model {
	return model{cfg: cfg, path: path, screen: screenList}
}

func (m model) Init() tea.Cmd { return nil }

// --- form helpers ----------------------------------------------------------

func fieldLabel(i int) string {
	switch i {
	case fName:
		return "Name"
	case fHost:
		return "Host"
	case fPort:
		return "Port"
	case fUser:
		return "User"
	case fPassword:
		return "Password"
	case fUseTLS:
		return "Use TLS"
	case fTLSSkipVerify:
		return "TLS skip verify"
	case fTimeout:
		return "Timeout (s)"
	}
	return ""
}

func isBoolField(i int) bool { return i == fUseTLS || i == fTLSSkipVerify }

func (m *model) startForm(p config.Profile, editing string) {
	m.inputs = make([]textinput.Model, fieldCount)
	for i := range m.inputs {
		ti := textinput.New()
		ti.CharLimit = 256
		ti.Width = 40
		switch i {
		case fName:
			ti.Placeholder = "home-router"
			ti.SetValue(p.Name)
		case fHost:
			ti.Placeholder = "192.168.88.1"
			ti.SetValue(p.Host)
		case fPort:
			ti.Placeholder = "8728 (8729 for TLS)"
			if p.Port != 0 {
				ti.SetValue(strconv.Itoa(p.Port))
			}
		case fUser:
			ti.Placeholder = "admin"
			ti.SetValue(p.User)
		case fPassword:
			ti.Placeholder = "(hidden)"
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
			ti.SetValue(p.Password)
		case fTimeout:
			ti.Placeholder = "30"
			if p.TimeoutSeconds != 0 {
				ti.SetValue(strconv.Itoa(p.TimeoutSeconds))
			}
		}
		m.inputs[i] = ti
	}
	m.bools[fUseTLS] = p.UseTLS
	m.bools[fTLSSkipVerify] = p.TLSSkipVerify
	m.editingOf = editing
	m.focus = 0
	m.errMsg = ""
	m.screen = screenForm
	m.applyFocus()
}

func (m *model) applyFocus() {
	for i := range m.inputs {
		if i == m.focus && !isBoolField(i) {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m *model) collectProfile() (config.Profile, error) {
	p := config.Profile{
		Name:          strings.TrimSpace(m.inputs[fName].Value()),
		Host:          strings.TrimSpace(m.inputs[fHost].Value()),
		User:          strings.TrimSpace(m.inputs[fUser].Value()),
		Password:      m.inputs[fPassword].Value(),
		UseTLS:        m.bools[fUseTLS],
		TLSSkipVerify: m.bools[fTLSSkipVerify],
	}
	if v := strings.TrimSpace(m.inputs[fPort].Value()); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return p, fmt.Errorf("port must be a number")
		}
		p.Port = n
	}
	if v := strings.TrimSpace(m.inputs[fTimeout].Value()); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return p, fmt.Errorf("timeout must be a number")
		}
		p.TimeoutSeconds = n
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	// Reject duplicate name when adding (or renaming onto another profile).
	if _, exists := m.cfg.Get(p.Name); exists && !strings.EqualFold(p.Name, m.editingOf) {
		return p, fmt.Errorf("a profile named %q already exists", p.Name)
	}
	return p, nil
}

// --- update ----------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenList:
		return m.updateList(msg)
	case screenForm:
		return m.updateForm(msg)
	case screenConfirmDelete:
		return m.updateConfirm(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.cfg.Profiles)-1 {
			m.cursor++
		}
	case "a":
		m.status = ""
		m.startForm(config.Profile{}, "")
	case "e", "enter":
		if len(m.cfg.Profiles) > 0 {
			p := m.cfg.Profiles[m.cursor]
			m.status = ""
			m.startForm(p, p.Name)
		}
	case "d":
		if len(m.cfg.Profiles) > 0 {
			m.screen = screenConfirmDelete
		}
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		name := m.cfg.Profiles[m.cursor].Name
		m.cfg.Remove(name)
		if m.cursor >= len(m.cfg.Profiles) && m.cursor > 0 {
			m.cursor--
		}
		if err := m.cfg.SaveTo(m.path); err != nil {
			m.errMsg = err.Error()
		} else {
			m.status = fmt.Sprintf("Deleted profile %q", name)
		}
		m.screen = screenList
	case "n", "N", "esc", "q":
		m.screen = screenList
	}
	return m, nil
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.screen = screenList
			m.errMsg = ""
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab", "down":
			m.focus = (m.focus + 1) % fieldCount
			m.applyFocus()
			return m, nil
		case "shift+tab", "up":
			m.focus = (m.focus - 1 + fieldCount) % fieldCount
			m.applyFocus()
			return m, nil
		case "ctrl+s":
			return m.save()
		case " ", "left", "right":
			if isBoolField(m.focus) {
				m.bools[m.focus] = !m.bools[m.focus]
				return m, nil
			}
		case "enter":
			// On the last field Enter saves; otherwise advance.
			if m.focus == fieldCount-1 {
				return m.save()
			}
			m.focus = (m.focus + 1) % fieldCount
			m.applyFocus()
			return m, nil
		}
	}

	if isBoolField(m.focus) {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m model) save() (tea.Model, tea.Cmd) {
	p, err := m.collectProfile()
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	// If renaming, drop the old entry first.
	if m.editingOf != "" && !strings.EqualFold(m.editingOf, p.Name) {
		m.cfg.Remove(m.editingOf)
	}
	m.cfg.Upsert(p)
	if err := m.cfg.SaveTo(m.path); err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	// Move cursor to the saved profile.
	for i, pp := range m.cfg.Profiles {
		if strings.EqualFold(pp.Name, p.Name) {
			m.cursor = i
			break
		}
	}
	if m.editingOf == "" {
		m.status = fmt.Sprintf("Added profile %q", p.Name)
	} else {
		m.status = fmt.Sprintf("Saved profile %q", p.Name)
	}
	m.screen = screenList
	return m, nil
}

// --- view ------------------------------------------------------------------

func (m model) View() string {
	if m.quitting {
		return ""
	}
	switch m.screen {
	case screenForm:
		return m.viewForm()
	case screenConfirmDelete:
		return m.viewConfirm()
	default:
		return m.viewList()
	}
}

func (m model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("MikroTik connection profiles") + "\n")
	b.WriteString(helpStyle.Render(m.path) + "\n\n")

	if len(m.cfg.Profiles) == 0 {
		b.WriteString(helpStyle.Render("  No profiles yet. Press 'a' to add one.") + "\n")
	}
	for i, p := range m.cfg.Profiles {
		cursor := "  "
		line := fmt.Sprintf("%s  %s@%s", p.Name, p.User, p.Host)
		if p.Port != 0 {
			line += fmt.Sprintf(":%d", p.Port)
		}
		if p.UseTLS {
			line += "  [tls]"
		}
		if i == m.cursor {
			cursor = selectedStyle.Render("> ")
			line = selectedStyle.Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(okStyle.Render(m.status) + "\n")
	}
	if m.errMsg != "" {
		b.WriteString(errStyle.Render("error: "+m.errMsg) + "\n")
	}
	b.WriteString(helpStyle.Render("↑/↓ move • a add • e/enter edit • d delete • q quit"))
	return b.String()
}

func (m model) viewForm() string {
	var b strings.Builder
	title := "Add profile"
	if m.editingOf != "" {
		title = "Edit profile: " + m.editingOf
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")

	for i := 0; i < fieldCount; i++ {
		label := fieldLabel(i)
		pointer := "  "
		if i == m.focus {
			pointer = focusedStyle.Render("> ")
		}
		var value string
		if isBoolField(i) {
			box := "[ ]"
			if m.bools[i] {
				box = "[x]"
			}
			value = box + "  " + helpStyle.Render("(space toggles)")
		} else {
			value = m.inputs[i].View()
		}
		b.WriteString(fmt.Sprintf("%s%-16s %s\n", pointer, label+":", value))
	}

	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString(errStyle.Render("error: "+m.errMsg) + "\n")
	}
	b.WriteString(helpStyle.Render("tab/↑↓ move • space toggle • enter next/save • ctrl+s save • esc cancel"))
	return b.String()
}

func (m model) viewConfirm() string {
	name := ""
	if m.cursor < len(m.cfg.Profiles) {
		name = m.cfg.Profiles[m.cursor].Name
	}
	return titleStyle.Render("Delete profile") + "\n\n" +
		fmt.Sprintf("  Delete profile %q? ", name) +
		helpStyle.Render("(y/n)")
}
