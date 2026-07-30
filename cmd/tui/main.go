// Command tui is the terminal frontend for the keymap trainer. It wraps the
// pure core.App (see core/app.go) in a Bubble Tea program: Update maps
// keypresses to core.Event values and calls Dispatch; View renders the
// returned core.State with Lip Gloss.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui: ", err)
		os.Exit(1)
	}
}
