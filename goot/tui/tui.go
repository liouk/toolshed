package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenPicker screen = iota
	screenCreator
)

// TaskAPI is the interface the TUI uses to talk to Google Tasks.
type TaskAPI interface {
	Lists(ctx context.Context) ([]TaskList, error)
	Tasks(ctx context.Context, listID string) ([]Task, error)
	CreateTask(ctx context.Context, listID string, task Task) error
}

type TaskList struct {
	ID    string
	Title string
}

type Task struct {
	Title string
	Notes string
	Due   string
}

// Config holds the TUI startup configuration.
// HideListFunc is called to persist a newly hidden list ID.
type HideListFunc func(listID string) error

// Config holds the TUI startup configuration.
type Config struct {
	API             TaskAPI
	ListName        string       // optional: skip picker if this matches a list
	HiddenListsByID []string     // list IDs to hide from the TUI
	HideList        HideListFunc // callback to persist hidden list
}

type model struct {
	cfg        Config
	screen     screen
	picker     pickerModel
	creator    creatorModel
	err        error
	quitting   bool
	submitting bool
	confirming bool // esc was pressed, waiting for y/n
	width      int
}

// listsLoadedMsg is sent when task lists have been fetched.
type listsLoadedMsg struct {
	lists []TaskList
}

// tasksLoadedMsg is sent when tasks for a list have been fetched.
type tasksLoadedMsg struct {
	tasks []Task
}

// taskCreatedMsg is sent when a task has been successfully created.
type taskCreatedMsg struct{}

// successTimeoutMsg is sent after the success message display duration.
type successTimeoutMsg struct{}

// clearStatusMsg clears the picker status message.
type clearStatusMsg struct{}

// errMsg wraps any error from async operations.
type errMsg struct{ err error }

func New(cfg Config) model {
	return model{
		cfg:    cfg,
		screen: screenPicker,
		picker: newPicker(),
	}
}

func (m model) Init() tea.Cmd {
	return m.loadLists()
}

func (m model) loadLists() tea.Cmd {
	return func() tea.Msg {
		lists, err := m.cfg.API.Lists(context.Background())
		if err != nil {
			return errMsg{err}
		}
		tl := make([]TaskList, len(lists))
		for i, l := range lists {
			tl[i] = TaskList{ID: l.ID, Title: l.Title}
		}
		return listsLoadedMsg{lists: tl}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		// Error screen: quit on q/esc.
		if m.err != nil {
			if msg.String() == "q" || msg.String() == "esc" {
				return m, tea.Quit
			}
			return m, nil
		}

		// Quit confirmation prompt.
		if m.confirming {
			if msg.String() == "y" {
				m.quitting = true
				return m, tea.Quit
			}
			m.confirming = false
			return m, nil
		}

		if msg.String() == "esc" && m.screen == screenCreator {
			m.confirming = true
			return m, nil
		}

		// Go back to picker when backspace is pressed on an empty title field.
		if msg.String() == "backspace" && m.screen == screenCreator &&
			m.creator.focus == fieldTitle && m.creator.inputs[fieldTitle].Value() == "" {
			m.screen = screenPicker
			m.picker.chosen = false
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.picker.list.SetWidth(msg.Width)
		m.creator.width = msg.Width
		return m, nil

	case listsLoadedMsg:
		lists := m.filterHidden(msg.lists)
		m.picker = pickerWithLists(m.picker, lists)

		// Skip picker if only one list exists.
		if len(lists) == 1 {
			return m.selectList(lists[0])
		}

		// If a list name was provided, try to skip the picker.
		if m.cfg.ListName != "" {
			for _, l := range lists {
				if matchesListName(l.Title, m.cfg.ListName) {
					return m.selectList(l)
				}
			}
		}
		return m, nil

	case tasksLoadedMsg:
		m.creator = creatorWithTasks(m.creator, msg.tasks)
		return m, nil

	case taskCreatedMsg:
		m.quitting = true
		return m, tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
			return successTimeoutMsg{}
		})

	case successTimeoutMsg:
		return m, tea.Quit

	case clearStatusMsg:
		m.picker.statusMsg = ""
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}

	switch m.screen {
	case screenPicker:
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)

		if m.picker.chosen {
			return m.selectList(m.picker.selected)
		}
		if m.picker.hiding {
			m.picker.hiding = false
			hidden := m.picker.hidden
			m.cfg.HiddenListsByID = append(m.cfg.HiddenListsByID, hidden.ID)

			// Rebuild the list without the hidden item.
			var filtered []TaskList
			for _, item := range m.picker.list.Items() {
				if li, ok := item.(listItem); ok && li.list.ID != hidden.ID {
					filtered = append(filtered, li.list)
				}
			}
			m.picker = pickerWithLists(m.picker, filtered)

			// Persist to config.
			if m.cfg.HideList != nil {
				if err := m.cfg.HideList(hidden.ID); err != nil {
					m.err = err
					return m, nil
				}
			}

			m.picker.statusMsg = fmt.Sprintf("Hidden: %s", hidden.Title)
			return m, tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}
		return m, cmd

	case screenCreator:
		if m.submitting {
			return m, nil
		}

		var cmd tea.Cmd
		m.creator, cmd = m.creator.Update(msg)

		if m.creator.submitted {
			m.submitting = true
			return m, m.createTask()
		}
		return m, cmd
	}

	return m, nil
}

func (m *model) selectList(list TaskList) (tea.Model, tea.Cmd) {
	m.screen = screenCreator
	m.creator = newCreator(list)
	m.creator.width = m.width
	return m, m.loadTasks(list.ID)
}

func (m model) loadTasks(listID string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := m.cfg.API.Tasks(context.Background(), listID)
		if err != nil {
			return errMsg{err}
		}
		tt := make([]Task, len(tasks))
		for i, t := range tasks {
			tt[i] = Task{Title: t.Title, Notes: t.Notes, Due: t.Due}
		}
		return tasksLoadedMsg{tasks: tt}
	}
}

func (m model) createTask() tea.Cmd {
	task := m.creator.task()
	listID := m.creator.list.ID
	return func() tea.Msg {
		err := m.cfg.API.CreateTask(context.Background(), listID, Task{
			Title: task.Title,
			Notes: task.Notes,
			Due:   task.Due,
		})
		if err != nil {
			return errMsg{err}
		}
		return taskCreatedMsg{}
	}
}

func (m model) View() string {
	if m.err != nil {
		errPrefix := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true).Render("Error:")
		return fmt.Sprintf("\n  %s %s\n\n  Press q/esc to quit.\n\n", errPrefix, m.err)
	}
	if m.quitting {
		return "\n  Task created!\n\n"
	}
	if m.confirming {
		return "\n  Quit without saving? (y/n)\n\n"
	}

	switch m.screen {
	case screenPicker:
		return m.picker.View()
	case screenCreator:
		return m.creator.View()
	}
	return ""
}

func (m model) filterHidden(lists []TaskList) []TaskList {
	if len(m.cfg.HiddenListsByID) == 0 {
		return lists
	}
	hidden := make(map[string]bool, len(m.cfg.HiddenListsByID))
	for _, id := range m.cfg.HiddenListsByID {
		hidden[id] = true
	}
	filtered := make([]TaskList, 0, len(lists))
	for _, l := range lists {
		if !hidden[l.ID] {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

func matchesListName(title, name string) bool {
	return len(title) > 0 && len(name) > 0 &&
		equalFold(title, name)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
