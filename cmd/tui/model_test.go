package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// TestSanity exercises construction, a representative key in each mode, and
// View() rendering, to guard against panics on startup/basic use. It does not
// attempt to drive the program interactively.
func TestSanity(t *testing.T) {
	m := newModel(core.NewDefault())
	if v := m.View(); !strings.Contains(v, "Mirror typing") {
		t.Fatalf("initial view missing tab bar: %q", v)
	}

	// Mirror mode: type a supported rune, an unsupported one, skip, and the
	// mode/length toggles.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = mm.(model)
	if m.state.Mode != core.ModeMirror {
		t.Fatalf("expected still in mirror mode, got %v", m.state.Mode)
	}
	_ = m.View()

	// Switch to nav via F2 and send an arrow.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = mm.(model)
	if m.state.Mode != core.ModeNav {
		t.Fatalf("expected nav mode, got %v", m.state.Mode)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = mm.(model)
	if v := m.View(); v == "" {
		t.Fatal("nav view empty")
	}

	// Switch to scratch via F3, type and backspace.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = mm.(model)
	if string(m.scratch) != "h" {
		t.Fatalf("expected scratch buffer %q, got %q", "h", string(m.scratch))
	}
	if v := m.View(); v == "" {
		t.Fatal("scratch view empty")
	}

	// Switch to reference via F4.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF4})
	m = mm.(model)
	if m.state.Mode != core.ModeReference {
		t.Fatalf("expected reference mode, got %v", m.state.Mode)
	}
	if v := m.View(); !strings.Contains(v, "Layer keys") {
		t.Fatalf("reference view missing layer key table: %q", v)
	}

	// ctrl+1..4 best-effort string match also switches modes.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	_ = mm

	// Quit bindings should return tea.Quit as the command.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c")
	}
}
