package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(4)).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	clonedStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
)
