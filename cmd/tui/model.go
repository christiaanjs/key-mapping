// Package main is the terminal frontend for the keymap trainer. It wraps the
// pure core.App in a Bubble Tea program: Update translates key messages into
// core.Event values and calls Dispatch; View renders the returned core.State
// with Lip Gloss.
package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/christiaanswanepoel/key-mapping/core"
)

// model is the Bubble Tea Model. It holds the core app plus the one piece of
// state the core deliberately does not own: the scratch-mode buffer, which
// exists purely to echo whatever the frontend receives (see core/app.go's
// handling of EvType: "ModeScratch ... ignores EvType: the frontend echoes
// the live keymap itself, with no server-side state.").
type model struct {
	app     *core.App
	state   core.State
	scratch []rune

	width, height int
}

// newModel builds the Bubble Tea model around the given core.App. Callers
// (main, tests) are responsible for constructing app with whatever mapping
// and corpus are appropriate.
func newModel(app *core.App) model {
	return model{
		app:   app,
		state: app.Snapshot(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) View() string {
	return renderView(m)
}

// handleKey routes a key message: first the global bindings (quit, mode
// switch), then mode-specific bindings.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyF1:
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeMirror}), nil
	case tea.KeyF2:
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeNav}), nil
	case tea.KeyF3:
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeScratch}), nil
	case tea.KeyF4:
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeReference}), nil
	}

	// Best-effort literal ctrl+1..ctrl+4: most terminals do not emit a
	// distinguishable byte sequence for Ctrl held with a digit (there is no
	// standard C0 control code for '1'-'4', unlike ctrl+letter), so
	// bubbletea v1.3 cannot represent it as its own KeyType. F1-F4 above are
	// the reliable binding; this switch only fires on terminals/setups that
	// do somehow forward "ctrl+1".."ctrl+4" as their key string.
	switch msg.String() {
	case "ctrl+1":
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeMirror}), nil
	case "ctrl+2":
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeNav}), nil
	case "ctrl+3":
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeScratch}), nil
	case "ctrl+4":
		return m.dispatch(core.Event{Type: core.EvSwitchMode, Mode: core.ModeReference}), nil
	}

	switch m.state.Mode {
	case core.ModeMirror:
		return m.handleMirrorKey(msg), nil
	case core.ModeNav:
		return m.handleNavKey(msg), nil
	case core.ModeScratch:
		return m.handleScratchKey(msg), nil
	default:
		return m, nil
	}
}

// dispatch applies ev to the core and stores the resulting state.
func (m model) dispatch(ev core.Event) model {
	m.state = m.app.Dispatch(ev)
	return m
}

func (m model) handleMirrorKey(msg tea.KeyMsg) model {
	switch msg.Type {
	case tea.KeyCtrlN: // skip current drill item
		return m.dispatch(core.Event{Type: core.EvSkip})
	case tea.KeyCtrlW: // toggle words/sentences
		next := core.ContentSentences
		if m.state.Content == core.ContentSentences {
			next = core.ContentWords
		}
		return m.dispatch(core.Event{Type: core.EvSetContent, Content: next})
	case tea.KeyCtrlL: // cycle length: short -> any -> long -> short
		return m.dispatch(core.Event{Type: core.EvSetLength, Length: nextLength(m.state.Length)})
	case tea.KeySpace:
		return m.dispatch(core.Event{Type: core.EvType, Rune: " ", AtMillis: nowMillis()})
	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			return m
		}
		return m.dispatch(core.Event{Type: core.EvType, Rune: string(msg.Runes[0]), AtMillis: nowMillis()})
	}
	return m
}

func (m model) handleNavKey(msg tea.KeyMsg) model {
	var arrow string
	switch msg.Type {
	case tea.KeyLeft:
		arrow = core.ArrowLeft
	case tea.KeyDown:
		arrow = core.ArrowDown
	case tea.KeyUp:
		arrow = core.ArrowUp
	case tea.KeyRight:
		arrow = core.ArrowRight
	default:
		return m
	}
	return m.dispatch(core.Event{Type: core.EvArrow, Arrow: arrow})
}

// handleScratchKey appends/deletes from the local scratch buffer. Scratch
// mode has no core-side state (see core/app.go) — it exists to show exactly
// what arrives from the keymap, so the TUI keeps its own buffer.
func (m model) handleScratchKey(msg tea.KeyMsg) model {
	switch msg.Type {
	case tea.KeyBackspace:
		if len(m.scratch) > 0 {
			m.scratch = m.scratch[:len(m.scratch)-1]
		}
	case tea.KeyEnter:
		m.scratch = append(m.scratch, '\n')
	case tea.KeySpace:
		m.scratch = append(m.scratch, ' ')
	case tea.KeyRunes:
		m.scratch = append(m.scratch, msg.Runes...)
	}
	return m
}

func nextLength(l core.Length) core.Length {
	switch l {
	case core.LengthShort:
		return core.LengthAny
	case core.LengthAny:
		return core.LengthLong
	default:
		return core.LengthShort
	}
}

func nowMillis() int64 { return time.Now().UnixMilli() }
