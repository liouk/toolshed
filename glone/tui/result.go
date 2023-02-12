package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type autoDismissMsg struct{}

type resultScreen struct {
	message string
	isError bool
}

func newResult(message string, isError bool) (resultScreen, tea.Cmd) {
	r := resultScreen{message: message, isError: isError}
	if !isError {
		return r, tea.Tick(time.Second, func(_ time.Time) tea.Msg {
			return autoDismissMsg{}
		})
	}
	return r, nil
}

func (r resultScreen) View() string {
	if r.isError {
		return fmt.Sprintf("\n  %s\n\n  %s",
			errorStyle.Render("✗ "+r.message),
			helpStyle.Render("press q to quit"),
		)
	}
	return fmt.Sprintf("\n  %s", successStyle.Render("✓ "+r.message))
}
