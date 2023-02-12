package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liouk/toolshed/goot/tui"
)

func main() {
	ctx := context.Background()

	httpClient, err := authenticate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth: %s\n", err)
		os.Exit(1)
	}

	client, err := NewClient(ctx, httpClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "api: %s\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 && (os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		return
	}

	// Subcommand: goot done [range]
	if len(os.Args) > 1 && os.Args[1] == "done" {
		args := os.Args[2:]
		if len(args) == 0 {
			args = []string{"this-week"}
		}
		if err := runDone(ctx, client, args); err != nil {
			fmt.Fprintf(os.Stderr, "done: %s\n", err)
			os.Exit(1)
		}
		return
	}

	var listName string
	if len(os.Args) > 2 && os.Args[1] == "list" {
		listName = os.Args[2]
	}

	appCfg := loadConfig()

	cfg := tui.Config{
		API:             &apiAdapter{client},
		ListName:        listName,
		HiddenListsByID: appCfg.HiddenListsByID,
		HideList:        appCfg.addHiddenList,
	}

	p := tea.NewProgram(tui.New(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %s\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`goot - A minimal TUI for Google Tasks

Usage:
  goot                    Open the TUI (list picker → task creator)
  goot list <list-name>   Skip picker, jump to task creation for matching list
  goot done [range]       Print completed tasks as markdown (default: this-week)
  goot help               Show this help message

Done ranges:
  Shortcuts:    today, yesterday, this-week, last-week,
                this-month, last-month, this-quarter, last-quarter,
                this-year, last-year
  Explicit:     --from YYYY-MM-DD --to YYYY-MM-DD

Examples:
  goot done this-month
  goot done last-quarter
  goot done --from 2026-01-01 --to 2026-03-31
  goot done last-month | claude -p "summarize what I worked on"
`)
}

// apiAdapter bridges the main package Client to the tui.TaskAPI interface.
type apiAdapter struct {
	c *Client
}

func (a *apiAdapter) Lists(ctx context.Context) ([]tui.TaskList, error) {
	lists, err := a.c.Lists(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]tui.TaskList, len(lists))
	for i, l := range lists {
		result[i] = tui.TaskList{ID: l.ID, Title: l.Title}
	}
	return result, nil
}

func (a *apiAdapter) Tasks(ctx context.Context, listID string) ([]tui.Task, error) {
	tasks, err := a.c.Tasks(ctx, listID)
	if err != nil {
		return nil, err
	}
	result := make([]tui.Task, len(tasks))
	for i, t := range tasks {
		result[i] = tui.Task{Title: t.Title, Notes: t.Notes, Due: t.Due}
	}
	return result, nil
}

func (a *apiAdapter) CreateTask(ctx context.Context, listID string, task tui.Task) error {
	return a.c.CreateTask(ctx, listID, Task{
		Title: task.Title,
		Notes: task.Notes,
		Due:   task.Due,
	})
}
