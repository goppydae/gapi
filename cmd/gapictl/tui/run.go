package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI
func Run() error {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
