package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	pickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.ANSIColor(4)). // blue
				Padding(0, 1)

	pickerItemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	pickerSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.ANSIColor(3)) // yellow
	pickerNumberStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))                // gray
)

type listItem struct {
	list TaskList
}

func (i listItem) FilterValue() string { return i.list.Title }

type listItemDelegate struct{}

func (d listItemDelegate) Height() int                             { return 1 }
func (d listItemDelegate) Spacing() int                            { return 0 }
func (d listItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d listItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	li, ok := item.(listItem)
	if !ok {
		return
	}

	style := pickerItemStyle
	prefix := ""
	if index == m.Index() {
		style = pickerSelectedItemStyle
		prefix = "> "
	}

	number := pickerNumberStyle.Render(fmt.Sprintf("%d.", index))
	fmt.Fprint(w, style.Render(fmt.Sprintf("%s%s %s", prefix, number, li.list.Title)))
}

type pickerModel struct {
	list      list.Model
	help      help.Model
	selected  TaskList
	chosen    bool
	hidden   TaskList
	hiding    bool
	statusMsg string
}

var statusStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).PaddingLeft(2)

type pickerKeyMap struct{}

func (k pickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "up/down")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "hide list")),
	}
}

func (k pickerKeyMap) FullHelp() [][]key.Binding { return nil }

func newPicker() pickerModel {
	l := list.New(nil, listItemDelegate{}, 40, 14)
	l.Title = "Select a task list"
	l.Styles.Title = pickerTitleStyle
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.KeyMap.CursorUp = key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "up"))
	l.KeyMap.CursorDown = key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "down"))
	return pickerModel{list: l, help: help.New()}
}

func pickerWithLists(p pickerModel, lists []TaskList) pickerModel {
	items := make([]list.Item, len(lists))
	for i, l := range lists {
		items[i] = listItem{list: l}
	}
	p.list.SetItems(items)
	// +6 accounts for title, spacing, filter input, help lines, and padding.
	p.list.SetHeight(len(lists) + 6)
	return p
}

func (m pickerModel) Update(msg tea.Msg) (pickerModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(listItem); ok {
				m.selected = item.list
				m.chosen = true
				return m, nil
			}
		case "x":
			if !m.list.SettingFilter() {
				if item, ok := m.list.SelectedItem().(listItem); ok {
					m.hidden = item.list
					m.hiding = true
					return m, nil
				}
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if !m.list.SettingFilter() {
				idx := int(key.String()[0] - '0')
				items := m.list.Items()
				if idx < len(items) {
					if item, ok := items[idx].(listItem); ok {
						m.selected = item.list
						m.chosen = true
						return m, nil
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(statusStyle.Render(m.statusMsg))
		b.WriteString("\n")
	}
	b.WriteString("  ")
	b.WriteString(m.help.View(pickerKeyMap{}))
	b.WriteString("\n")
	return b.String()
}
