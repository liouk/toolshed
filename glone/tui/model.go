package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenRepo screen = iota
	screenResult
)

type reposCachedMsg struct {
	org   string
	repos []repoItem
}

type reposLoadedMsg struct {
	org   string
	repos []repoItem
}

type reposErrorMsg struct {
	org string
	err error
}

type cloneDoneMsg struct {
	path       string
	repoName   string
	message    string
	err        error
	openEditor bool
}

type editorOpenedMsg struct {
	path    string
	message string
	err     error
}

type Org struct {
	Name          string
	CloneDir      string
	ForkCloneDirs map[string]string
	Exclude       map[string]bool
}

type Model struct {
	screen     screen
	repoPicker repoPicker
	result     resultScreen
	orgs       []Org
	editor     string
	quitting   bool
	resultText string
}

func New(orgs []Org, editor string) Model {
	rp := newRepoPicker()
	rp.orgItems = make(map[string][]repoItem)
	rp.pendingFetch = len(orgs)
	return Model{
		orgs:       orgs,
		editor:     editor,
		screen:     screenRepo,
		repoPicker: rp,
	}
}

func (m Model) Result() string {
	return m.resultText
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.repoPicker.spinner.Tick}
	for _, org := range m.orgs {
		cmds = append(cmds,
			loadCachedRepos(org.Name, org.CloneDir, org.ForkCloneDirs, org.Exclude),
			fetchRepos(org.Name, org.CloneDir, org.ForkCloneDirs, org.Exclude),
		)
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.screen != screenRepo {
				m.quitting = true
				return m, tea.Quit
			}
		}

	case reposCachedMsg:
		if msg.repos != nil {
			m.repoPicker.orgItems[msg.org] = msg.repos
			m.repoPicker.rebuildAllItems()
		}
		if len(m.repoPicker.allItems) > 0 {
			m.repoPicker.loading = false
			m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
		}
		return m, nil

	case reposLoadedMsg:
		m.repoPicker.pendingFetch--
		m.repoPicker.orgItems[msg.org] = msg.repos
		m.repoPicker.rebuildAllItems()
		m.repoPicker.loading = false
		m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
		return m, nil

	case reposErrorMsg:
		m.repoPicker.pendingFetch--
		if len(m.repoPicker.allItems) > 0 || m.repoPicker.pendingFetch > 0 {
			m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
			return m, nil
		}
		m.screen = screenResult
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.err.Error(), true)
		return m, cmd

	case cloneDoneMsg:
		if msg.err != nil {
			m.screen = screenResult
			var cmd tea.Cmd
			m.result, cmd = newResult(msg.err.Error(), true)
			return m, cmd
		}
		if msg.openEditor && m.editor != "" {
			return m, m.openEditor(repoItem{name: msg.repoName}, msg.path)
		}
		m.screen = screenResult
		m.resultText = msg.path
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.message, false)
		return m, cmd

	case editorOpenedMsg:
		if msg.err != nil {
			m.screen = screenResult
			var cmd tea.Cmd
			m.result, cmd = newResult(msg.err.Error(), true)
			return m, cmd
		}
		m.screen = screenResult
		m.resultText = msg.path
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.message, false)
		return m, cmd

	case autoDismissMsg:
		m.quitting = true
		return m, tea.Quit
	}

	switch m.screen {
	case screenRepo:
		return m.updateRepo(msg)
	case screenResult:
		return m, nil
	}

	return m, nil
}

func (m Model) updateRepo(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.repoPicker, cmd = m.repoPicker.Update(msg)

	if m.repoPicker.action.kind != actionNone {
		action := m.repoPicker.action
		m.repoPicker.action = repoAction{}

		switch action.kind {
		case actionBrowser:
			return m, m.openBrowser(action.item)
		case actionDeepClone, actionShallowClone:
			m.repoPicker.loading = true
			m.repoPicker.loadingMsg = fmt.Sprintf("Cloning %s…", action.item.name)
			return m, tea.Batch(
				m.repoPicker.spinner.Tick,
				m.doClone(action),
			)
		case actionOpen:
			return m, m.openEditor(action.item, "")
		case actionFork:
			m.repoPicker.loading = true
			m.repoPicker.loadingMsg = fmt.Sprintf("Forking %s…", action.item.name)
			return m, tea.Batch(
				m.repoPicker.spinner.Tick,
				m.doFork(action.item),
			)
		}
	}

	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.screen {
	case screenRepo:
		return m.repoPicker.View()
	case screenResult:
		return m.result.View()
	}
	return ""
}

func cachedReposToItems(org string, repos []cachedRepo, cloneDir string, forkCloneDirs map[string]string, exclude map[string]bool) []repoItem {
	var items []repoItem
	for _, r := range repos {
		if exclude[r.Name] {
			continue
		}
		dir := resolveCloneDir(cloneDir, forkCloneDirs, r.ParentOrg)
		items = append(items, repoItem{
			org:         org,
			name:        r.Name,
			url:         r.URL,
			description: r.Description,
			parentOrg:   r.ParentOrg,
			cloned:      isRepoCloned(dir, r.Name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].name < items[j].name
	})
	return items
}

func loadCachedRepos(org, cloneDir string, forkCloneDirs map[string]string, exclude map[string]bool) tea.Cmd {
	return func() tea.Msg {
		repos, err := readCache(org)
		if err != nil || len(repos) == 0 {
			return nil
		}
		return reposCachedMsg{org: org, repos: cachedReposToItems(org, repos, cloneDir, forkCloneDirs, exclude)}
	}
}

func fetchRepos(org, cloneDir string, forkCloneDirs map[string]string, exclude map[string]bool) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("gh", "repo", "list", org,
			"--json", "name,url,description,isFork,parent",
			"--no-archived",
			"--limit", "1000",
		)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return reposErrorMsg{org: org, err: fmt.Errorf("gh: %s", string(exitErr.Stderr))}
			}
			return reposErrorMsg{org: org, err: err}
		}

		type ghRepo struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			Description string `json:"description"`
			IsFork      bool   `json:"isFork"`
			Parent      *struct {
				Owner struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"parent"`
		}

		var repos []ghRepo
		if err := jsonUnmarshal(out, &repos); err != nil {
			return reposErrorMsg{org: org, err: err}
		}

		cached := make([]cachedRepo, len(repos))
		for i, r := range repos {
			parentOrg := ""
			if r.IsFork && r.Parent != nil {
				parentOrg = r.Parent.Owner.Login
			}
			cached[i] = cachedRepo{
				Name:        r.Name,
				URL:         r.URL,
				Description: r.Description,
				IsFork:      r.IsFork,
				ParentOrg:   parentOrg,
			}
		}
		writeCache(org, cached)

		return reposLoadedMsg{org: org, repos: cachedReposToItems(org, cached, cloneDir, forkCloneDirs, exclude)}
	}
}

func resolveCloneDir(cloneDir string, forkCloneDirs map[string]string, parentOrg string) string {
	if parentOrg != "" {
		if dir, ok := forkCloneDirs[parentOrg]; ok {
			return dir
		}
	}
	return cloneDir
}

func (m Model) orgConfig(name string) Org {
	for _, o := range m.orgs {
		if o.Name == name {
			return o
		}
	}
	return Org{}
}

func (m Model) cloneDirFor(item repoItem) string {
	org := m.orgConfig(item.org)
	return resolveCloneDir(org.CloneDir, org.ForkCloneDirs, item.parentOrg)
}

func (m Model) doClone(action repoAction) tea.Cmd {
	return func() tea.Msg {
		shallow := action.kind == actionShallowClone
		dir := m.cloneDirFor(action.item)
		path, err := cloneRepoCmd(action.item.url, dir, action.item.name, shallow)
		if err != nil {
			return cloneDoneMsg{err: err}
		}
		kind := "cloned"
		if shallow {
			kind = "shallow cloned"
		}
		return cloneDoneMsg{
			path:       path,
			repoName:   action.item.name,
			message:    fmt.Sprintf("%s to %s", kind, path),
			openEditor: true,
		}
	}
}

func (m Model) doFork(item repoItem) tea.Cmd {
	return func() tea.Msg {
		// get authenticated GitHub username
		userCmd := exec.Command("gh", "api", "user", "--jq", ".login")
		userOut, err := userCmd.Output()
		if err != nil {
			return cloneDoneMsg{err: fmt.Errorf("could not get GitHub user: %w", err)}
		}
		ghUser := strings.TrimSpace(string(userOut))

		// find the user's org config to resolve clone dir
		var userOrg *Org
		for i := range m.orgs {
			if m.orgs[i].Name == ghUser {
				userOrg = &m.orgs[i]
				break
			}
		}
		if userOrg == nil {
			return cloneDoneMsg{err: fmt.Errorf("no org config found for your GitHub user %q", ghUser)}
		}

		// resolve clone dir: check fork_clone_dirs for the repo's org
		cloneDir := userOrg.CloneDir
		if dir, ok := userOrg.ForkCloneDirs[item.org]; ok {
			cloneDir = dir
		}

		// fork the repo
		forkCmd := exec.Command("gh", "repo", "fork", item.org+"/"+item.name, "--clone=false")
		if out, err := forkCmd.CombinedOutput(); err != nil {
			return cloneDoneMsg{err: fmt.Errorf("fork failed: %s", string(out))}
		}

		// clone the fork
		forkURL := fmt.Sprintf("git@github.com:%s/%s.git", ghUser, item.name)
		path, err := cloneRepoCmd(forkURL, cloneDir, item.name, false)
		if err != nil {
			return cloneDoneMsg{err: err}
		}

		return cloneDoneMsg{
			path:       path,
			repoName:   item.name,
			message:    fmt.Sprintf("forked and cloned to %s", path),
			openEditor: true,
		}
	}
}

func (m Model) openBrowser(item repoItem) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("gh", "browse", "-R", item.org+"/"+item.name)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return cloneDoneMsg{err: fmt.Errorf("could not open browser: %w", err)}
		}
		return cloneDoneMsg{message: "opened in browser"}
	}
}

func (m Model) openEditor(item repoItem, path string) tea.Cmd {
	return func() tea.Msg {
		if path == "" {
			dir := m.cloneDirFor(item)
			path = filepath.Join(dir, item.name)
		}
		_, err := openEditorCmd(m.editor, filepath.Dir(path), filepath.Base(path))
		if err != nil {
			return editorOpenedMsg{err: err}
		}
		return editorOpenedMsg{
			path:    path,
			message: fmt.Sprintf("opened %s", path),
		}
	}
}
