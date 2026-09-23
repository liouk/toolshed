package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenRepo screen = iota
	screenCloneForPR
	screenPullRequests
	screenOpenPrompt
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
	loadPulls  bool
	item       repoItem
}

type cloneOutputMsg struct {
	line    string
	replace bool
}

type editorOpenedMsg struct {
	path    string
	message string
	err     error
}

type pullRequestsLoadedMsg struct {
	pulls []prItem
}

type pullRequestsErrorMsg struct {
	err error
}

type Org struct {
	Name                string
	CloneDir            string
	WorktreeDirTemplate string
	ForkCloneDirs       map[string]string
	Exclude             map[string]bool
}

type Model struct {
	screen          screen
	repoPicker      repoPicker
	prPicker        prPicker
	prRepo          repoItem
	result          resultScreen
	orgs            []Org
	editor          string
	quitting        bool
	resultText      string
	cloneEvents     <-chan tea.Msg
	pendingOpenPath string
	pendingOpenRepo string
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
		if m.screen == screenOpenPrompt && msg.String() != "esc" && msg.String() != "ctrl+c" {
			if msg.String() == "enter" {
				path := m.pendingOpenPath
				repoName := m.pendingOpenRepo
				m.pendingOpenPath = ""
				m.pendingOpenRepo = ""
				m.screen = screenRepo
				m.repoPicker.loading = true
				m.repoPicker.loadingMsg = fmt.Sprintf("Opening %s…", repoName)
				return m, m.openEditor(repoItem{name: repoName}, path)
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.screen == screenPullRequests {
				m.screen = screenRepo
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.screen != screenRepo && m.screen != screenPullRequests {
				m.quitting = true
				return m, tea.Quit
			}
		}

	case reposCachedMsg:
		if msg.repos != nil {
			m.repoPicker.orgItems[msg.org] = msg.repos
			m.repoPicker.rebuildAllItems()
		}
		if len(m.repoPicker.allItems) > 0 && m.repoPicker.loadingMsg == "" {
			m.repoPicker.loading = false
			m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
		}
		return m, nil

	case reposLoadedMsg:
		m.repoPicker.pendingFetch--
		m.repoPicker.orgItems[msg.org] = msg.repos
		m.repoPicker.rebuildAllItems()
		if m.repoPicker.loadingMsg == "" {
			m.repoPicker.loading = false
			m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
		}
		return m, nil

	case reposErrorMsg:
		m.repoPicker.pendingFetch--
		if m.repoPicker.loadingMsg != "" {
			return m, nil
		}
		if len(m.repoPicker.allItems) > 0 || m.repoPicker.pendingFetch > 0 {
			m.repoPicker.refreshing = m.repoPicker.pendingFetch > 0
			return m, nil
		}
		m.screen = screenResult
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.err.Error(), true)
		return m, cmd

	case cloneDoneMsg:
		m.cloneEvents = nil
		if msg.err != nil {
			m.screen = screenResult
			var cmd tea.Cmd
			m.result, cmd = newResult(msg.err.Error(), true)
			return m, cmd
		}
		if msg.loadPulls {
			m.prRepo = msg.item
			m.prRepo.cloned = true
			m.prPicker = newPRPicker()
			m.screen = screenPullRequests
			return m, tea.Batch(m.prPicker.spinner.Tick, fetchPullRequests(msg.item))
		}
		if msg.openEditor && m.editor != "" {
			m.pendingOpenPath = msg.path
			m.pendingOpenRepo = msg.repoName
			m.screen = screenOpenPrompt
			return m, nil
		}
		m.screen = screenResult
		m.resultText = msg.path
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.message, false)
		return m, cmd

	case cloneOutputMsg:
		m.repoPicker.addCloneOutput(msg.line, msg.replace)
		if m.cloneEvents != nil {
			return m, waitForCloneEvent(m.cloneEvents)
		}
		return m, nil

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

	switch msg := msg.(type) {
	case pullRequestsLoadedMsg:
		m.prPicker.loading = false
		m.prPicker.allItems = msg.pulls
		m.prPicker.applyFilter()
		return m, nil
	case pullRequestsErrorMsg:
		m.screen = screenResult
		var cmd tea.Cmd
		m.result, cmd = newResult(msg.err.Error(), true)
		return m, cmd
	}

	switch m.screen {
	case screenRepo:
		return m.updateRepo(msg)
	case screenCloneForPR:
		return m.updateCloneForPR(msg)
	case screenPullRequests:
		return m.updatePullRequests(msg)
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
			m.repoPicker.cloneOutput = nil
			m.repoPicker.cloneProgressActive = false
			events := make(chan tea.Msg, 64)
			m.cloneEvents = events
			return m, tea.Batch(
				m.repoPicker.spinner.Tick,
				m.doClone(action, events),
				waitForCloneEvent(events),
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
		case actionPullRequests:
			if !action.item.cloned {
				m.prRepo = action.item
				m.screen = screenCloneForPR
				return m, nil
			}
			m.prRepo = action.item
			m.prPicker = newPRPicker()
			m.screen = screenPullRequests
			return m, tea.Batch(m.prPicker.spinner.Tick, fetchPullRequests(action.item))
		}
	}

	return m, cmd
}

func (m Model) updateCloneForPR(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter", "y":
		m.screen = screenRepo
		m.repoPicker.loading = true
		m.repoPicker.loadingMsg = fmt.Sprintf("Cloning %s…", m.prRepo.name)
		m.repoPicker.cloneOutput = nil
		events := make(chan tea.Msg, 64)
		m.cloneEvents = events
		action := repoAction{kind: actionDeepClone, item: m.prRepo, loadPulls: true}
		return m, tea.Batch(m.repoPicker.spinner.Tick, m.doClone(action, events), waitForCloneEvent(events))
	case "n", "esc", "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updatePullRequests(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.prPicker, cmd = m.prPicker.Update(msg)
	if m.prPicker.selected == nil {
		return m, cmd
	}
	pull := *m.prPicker.selected
	m.prPicker.selected = nil
	m.prPicker.loading = true
	m.prPicker.loadingMsg = fmt.Sprintf("Creating worktree for #%d…", pull.number)
	return m, tea.Batch(m.prPicker.spinner.Tick, m.createWorktree(m.prRepo, pull))
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	switch m.screen {
	case screenRepo:
		return m.repoPicker.View()
	case screenCloneForPR:
		return fmt.Sprintf("\n  %s is not cloned.\n\n  Do you want to clone it first? [Y/n]\n\n%s",
			m.prRepo.org+"/"+m.prRepo.name,
			helpStyle.Render("  y/enter clone first • n/q/esc/ctrl-c quit"),
		)
	case screenPullRequests:
		return m.prPicker.View(m.prRepo)
	case screenOpenPrompt:
		return fmt.Sprintf("\n  %s\n\n%s",
			successStyle.Render("✓ "+fmt.Sprintf("ready at %s", m.pendingOpenPath)),
			helpStyle.Render(fmt.Sprintf("  press enter to open in %s • esc to quit", m.editor)),
		)
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

func fetchPullRequests(item repoItem) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("gh", "pr", "list", "-R", item.org+"/"+item.name,
			"--state", "open", "--json", "number,title,author", "--limit", "1000")
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return pullRequestsErrorMsg{err: fmt.Errorf("gh: %s", strings.TrimSpace(string(exitErr.Stderr)))}
			}
			return pullRequestsErrorMsg{err: fmt.Errorf("could not list pull requests: %w", err)}
		}
		var pulls []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		if err := jsonUnmarshal(out, &pulls); err != nil {
			return pullRequestsErrorMsg{err: fmt.Errorf("could not parse pull requests: %w", err)}
		}
		items := make([]prItem, len(pulls))
		for i, pull := range pulls {
			author := ""
			if pull.Author != nil {
				author = pull.Author.Login
			}
			items[i] = prItem{number: pull.Number, title: pull.Title, author: author}
		}
		return pullRequestsLoadedMsg{pulls: items}
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

func (m Model) doClone(action repoAction, events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			defer close(events)

			shallow := action.kind == actionShallowClone
			dir := m.cloneDirFor(action.item)
			path, err := cloneRepoCmd(action.item.url, dir, action.item.name, shallow, func(line string, replace bool) {
				events <- cloneOutputMsg{line: line, replace: replace}
			})
			if err != nil {
				events <- cloneDoneMsg{err: err}
				return
			}
			kind := "cloned"
			if shallow {
				kind = "shallow cloned"
			}
			events <- cloneDoneMsg{
				path:       path,
				repoName:   action.item.name,
				message:    fmt.Sprintf("%s to %s", kind, path),
				openEditor: true,
				loadPulls:  action.loadPulls,
				item:       action.item,
			}
		}()
		return nil
	}
}

func (m Model) createWorktree(item repoItem, pull prItem) tea.Cmd {
	return func() tea.Msg {
		base := filepath.Join(m.cloneDirFor(item), item.name)
		dest, err := m.worktreeDirFor(item, pull)
		if err != nil {
			return cloneDoneMsg{err: err}
		}
		if _, err := os.Stat(dest); err == nil {
			return cloneDoneMsg{err: fmt.Errorf("worktree already exists at %s", dest)}
		} else if !os.IsNotExist(err) {
			return cloneDoneMsg{err: fmt.Errorf("could not check worktree destination: %w", err)}
		}
		fetch := exec.Command("git", "-C", base, "fetch", "origin", fmt.Sprintf("pull/%d/head", pull.number))
		if out, err := fetch.CombinedOutput(); err != nil {
			return cloneDoneMsg{err: fmt.Errorf("could not fetch PR #%d: %s", pull.number, strings.TrimSpace(string(out)))}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return cloneDoneMsg{err: fmt.Errorf("could not create worktree directory: %w", err)}
		}
		branch := fmt.Sprintf("glone/pr-%d", pull.number)
		add := exec.Command("git", "-C", base, "worktree", "add", "-B", branch, dest, "FETCH_HEAD")
		if out, err := add.CombinedOutput(); err != nil {
			return cloneDoneMsg{err: fmt.Errorf("could not create worktree for PR #%d: %s", pull.number, strings.TrimSpace(string(out)))}
		}
		return cloneDoneMsg{
			path:       dest,
			repoName:   item.name,
			message:    fmt.Sprintf("created worktree for PR #%d at %s", pull.number, dest),
			openEditor: true,
		}
	}
}

func (m Model) worktreeDirFor(item repoItem, pull prItem) (string, error) {
	pattern := m.orgConfig(item.org).WorktreeDirTemplate
	if pattern == "" {
		pattern = "{{.clone_dir}}/{{.repo}}.wt/pr-{{.pr_num}}"
	}
	tmpl, err := template.New("worktree directory").Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid worktree_dir_template: %w", err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, map[string]any{
		"clone_dir": m.cloneDirFor(item),
		"repo":      item.name,
		"pr_num":    pull.number,
	}); err != nil {
		return "", fmt.Errorf("could not render worktree_dir_template: %w", err)
	}
	dir := filepath.Clean(rendered.String())
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("worktree_dir_template must render an absolute path, got %q", dir)
	}
	return dir, nil
}

func waitForCloneEvent(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
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
		path, err := cloneRepoCmd(forkURL, cloneDir, item.name, false, nil)
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
