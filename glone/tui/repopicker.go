package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type repoItem struct {
	org         string
	name        string
	url         string
	description string
	parentOrg   string
	cloned      bool
}

func (r repoItem) FilterValue() string { return r.org + "/" + r.name }
func (r repoItem) Title() string       { return r.org + "/" + r.name }
func (r repoItem) Description() string { return r.description }

type repoDelegate struct{}

func (d repoDelegate) Height() int                             { return 1 }
func (d repoDelegate) Spacing() int                            { return 0 }
func (d repoDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d repoDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	r := item.(repoItem)

	cloneCol := "  "
	if r.cloned {
		cloneCol = clonedStyle.Render("✓ ")
	}

	forkCol := "  "
	if r.parentOrg != "" {
		forkCol = dimStyle.Render("⑂ ")
	}

	nameStr := r.org + "/" + r.name
	if index == m.Index() {
		nameStr = selectedStyle.Render(nameStr)
	} else if r.cloned {
		nameStr = dimStyle.Render(r.org+"/") + clonedStyle.Render(r.name)
	} else {
		nameStr = dimStyle.Render(r.org+"/") + r.name
	}

	desc := ""
	if r.description != "" {
		desc = " " + dimStyle.Render(r.description)
		maxDesc := 60
		if len(r.description) > maxDesc {
			desc = " " + dimStyle.Render(r.description[:maxDesc]+"…")
		}
	}

	fmt.Fprintf(w, "%s%s%s%s", cloneCol, forkCol, nameStr, desc)
}

type repoPicker struct {
	list         list.Model
	filter       textinput.Model
	allItems     []repoItem
	orgItems     map[string][]repoItem
	spinner      spinner.Model
	loading      bool
	refreshing   bool
	loadingMsg   string
	pendingFetch int
	action       repoAction
}

type repoAction struct {
	kind actionKind
	item repoItem
}

type actionKind int

const (
	actionNone actionKind = iota
	actionDeepClone
	actionShallowClone
	actionBrowser
	actionOpen
	actionFork
)

func newRepoPicker() repoPicker {
	s := spinner.New()
	s.Spinner = spinner.Dot

	l := list.New(nil, repoDelegate{}, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(true)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	f := textinput.New()
	f.Prompt = "  Filter: "
	f.PromptStyle = dimStyle
	f.Focus()

	return repoPicker{
		list:    l,
		filter:  f,
		spinner: s,
		loading: true,
	}
}

func (r repoPicker) Update(msg tea.Msg) (repoPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			item, ok := r.list.SelectedItem().(repoItem)
			if !ok {
				break
			}
			if item.cloned {
				r.action = repoAction{kind: actionOpen, item: item}
			} else {
				r.action = repoAction{kind: actionDeepClone, item: item}
			}
			return r, nil
		case "ctrl+enter":
			item, ok := r.list.SelectedItem().(repoItem)
			if !ok {
				break
			}
			if !item.cloned {
				r.action = repoAction{kind: actionShallowClone, item: item}
			}
			return r, nil
		case "ctrl+f":
			item, ok := r.list.SelectedItem().(repoItem)
			if !ok {
				break
			}
			r.action = repoAction{kind: actionFork, item: item}
			return r, nil
		case "ctrl+o":
			item, ok := r.list.SelectedItem().(repoItem)
			if !ok {
				break
			}
			r.action = repoAction{kind: actionBrowser, item: item}
			return r, nil
		case "up", "ctrl+p":
			r.list, _ = r.list.Update(msg)
			return r, nil
		case "down", "ctrl+n":
			r.list, _ = r.list.Update(msg)
			return r, nil
		}

		// update filter input, then re-filter the list
		prevFilter := r.filter.Value()
		r.filter, _ = r.filter.Update(msg)
		if r.filter.Value() != prevFilter {
			r.applyFilter()
			return r, nil
		}
		return r, nil

	case spinner.TickMsg:
		if r.loading || r.refreshing {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			return r, cmd
		}

	case tea.WindowSizeMsg:
		r.list.SetSize(msg.Width, msg.Height-4)
	}

	var cmd tea.Cmd
	r.list, cmd = r.list.Update(msg)
	return r, cmd
}

func (r *repoPicker) applyFilter() {
	query := strings.ToLower(r.filter.Value())
	if query == "" {
		items := make([]list.Item, len(r.allItems))
		for i, item := range r.allItems {
			items[i] = item
		}
		r.list.SetItems(items)
		return
	}

	var filtered []list.Item
	for _, item := range r.allItems {
		if fuzzyMatch(strings.ToLower(item.org+"/"+item.name), query) {
			filtered = append(filtered, item)
		}
	}
	r.list.SetItems(filtered)
}

func (r *repoPicker) rebuildAllItems() {
	var all []repoItem
	for _, items := range r.orgItems {
		all = append(all, items...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].org != all[j].org {
			return all[i].org < all[j].org
		}
		return all[i].name < all[j].name
	})
	r.allItems = all
	r.applyFilter()
}

func fuzzyMatch(s, query string) bool {
	qi := 0
	for i := 0; i < len(s) && qi < len(query); i++ {
		if s[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

func (r repoPicker) View() string {
	if r.loading {
		msg := r.loadingMsg
		if msg == "" {
			msg = "Loading repos…"
		}
		return fmt.Sprintf("\n  %s %s", r.spinner.View(), msg)
	}

	var b strings.Builder
	b.WriteString(r.filter.View())
	if r.refreshing {
		b.WriteString("  ")
		b.WriteString(r.spinner.View())
		b.WriteString(dimStyle.Render(" refreshing…"))
	}
	b.WriteString("\n")
	b.WriteString(r.list.View())
	b.WriteString("\n")

	help := "  enter open/clone • ctrl+enter shallow clone • ctrl+f fork • ctrl+o browser • esc quit"
	b.WriteString(helpStyle.Render(help))
	return b.String()
}
