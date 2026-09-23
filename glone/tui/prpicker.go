package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type prItem struct {
	number int
	title  string
	author string
}

func (p prItem) FilterValue() string { return fmt.Sprintf("%d %s %s", p.number, p.title, p.author) }
func (p prItem) Title() string       { return fmt.Sprintf("#%d %s", p.number, p.title) }
func (p prItem) Description() string { return p.author }

type prDelegate struct{}

func (d prDelegate) Height() int                             { return 1 }
func (d prDelegate) Spacing() int                            { return 0 }
func (d prDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d prDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	p := item.(prItem)
	label := fmt.Sprintf("#%-5d %s", p.number, p.title)
	if index == m.Index() {
		label = selectedStyle.Render(label)
	}
	if p.author != "" {
		label += " " + dimStyle.Render("@"+p.author)
	}
	fmt.Fprint(w, label)
}

type prPicker struct {
	list       list.Model
	filter     textinput.Model
	allItems   []prItem
	spinner    spinner.Model
	loading    bool
	loadingMsg string
	selected   *prItem
}

func newPRPicker() prPicker {
	s := spinner.New()
	s.Spinner = spinner.Dot
	l := list.New(nil, prDelegate{}, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(true)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	f := textinput.New()
	f.Prompt = "  Filter: "
	f.PromptStyle = dimStyle
	f.Focus()
	return prPicker{list: l, filter: f, spinner: s, loading: true}
}

func (p prPicker) Update(msg tea.Msg) (prPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			item, ok := p.list.SelectedItem().(prItem)
			if ok {
				p.selected = &item
			}
			return p, nil
		case "up", "ctrl+p", "down", "ctrl+n":
			p.list, _ = p.list.Update(msg)
			return p, nil
		}
		previous := p.filter.Value()
		p.filter, _ = p.filter.Update(msg)
		if p.filter.Value() != previous {
			p.applyFilter()
		}
		return p, nil
	case spinner.TickMsg:
		if p.loading {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			return p, cmd
		}
	case tea.WindowSizeMsg:
		p.list.SetSize(msg.Width, msg.Height-4)
	}
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p *prPicker) applyFilter() {
	query := strings.ToLower(p.filter.Value())
	items := make([]list.Item, 0, len(p.allItems))
	if query == "" {
		for _, item := range p.allItems {
			items = append(items, item)
		}
	} else {
		targets := make([]string, len(p.allItems))
		for i, item := range p.allItems {
			targets[i] = strings.ToLower(item.FilterValue())
		}
		for _, rank := range list.DefaultFilter(query, targets) {
			items = append(items, p.allItems[rank.Index])
		}
	}
	p.list.SetItems(items)
}

func (p prPicker) View(repo repoItem) string {
	if p.loading {
		message := p.loadingMsg
		if message == "" {
			message = "Loading open pull requests…"
		}
		return fmt.Sprintf("\n  %s %s", p.spinner.View(), message)
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(repo.org + "/" + repo.name + " open pull requests"))
	b.WriteString("\n")
	b.WriteString(p.filter.View())
	b.WriteString("\n")
	b.WriteString(p.list.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  enter create worktree • esc back"))
	return b.String()
}
