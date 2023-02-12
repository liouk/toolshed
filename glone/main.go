package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liouk/toolshed/glone/tui"
)

func main() {
	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Fprintf(os.Stderr, "error: gh CLI not found; install it from https://cli.github.com\n")
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	orgs := make([]tui.Org, len(cfg.Orgs))
	for i, o := range cfg.Orgs {
		orgs[i] = tui.Org{Name: o.Name, CloneDir: o.CloneDir, ForkCloneDirs: o.ForkCloneDirs, Exclude: o.Exclude}
	}

	p := tea.NewProgram(tui.New(orgs, cfg.Editor), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if result := m.(tui.Model).Result(); result != "" {
		fmt.Println(result)
	}
}
