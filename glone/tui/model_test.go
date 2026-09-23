package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRefreshDoesNotDismissCloneLoadingState(t *testing.T) {
	m := New([]Org{{Name: "openshift"}}, "zed")
	m.repoPicker.loading = true
	m.repoPicker.loadingMsg = "Cloning origin…"
	m.repoPicker.pendingFetch = 1

	updated, _ := m.Update(reposLoadedMsg{org: "openshift"})
	got := updated.(Model)

	if !got.repoPicker.loading {
		t.Fatal("repository refresh dismissed clone loading state")
	}
	if got.repoPicker.loadingMsg != "Cloning origin…" {
		t.Fatalf("clone loading message changed: %q", got.repoPicker.loadingMsg)
	}
}

func TestPullRequestActionPromptsToCloneAnUnclonedRepository(t *testing.T) {
	m := New([]Org{{Name: "example", CloneDir: "/tmp"}}, "zed")
	item := repoItem{org: "example", name: "project"}
	m.repoPicker.action = repoAction{kind: actionPullRequests, item: item}

	updated, _ := m.updateRepo(nil)
	got := updated.(Model)
	if got.screen != screenCloneForPR {
		t.Fatalf("screen = %v, want clone confirmation", got.screen)
	}
	if got.prRepo != item {
		t.Fatalf("PR repository = %#v, want %#v", got.prRepo, item)
	}
	view := got.View()
	if !strings.Contains(view, "Do you want to clone it first? [Y/n]") || !strings.Contains(view, "y/enter clone first") || !strings.Contains(view, "n/q/esc/ctrl-c quit") {
		t.Fatalf("unexpected clone confirmation: %q", view)
	}
}

func TestCloneForPullRequestsOpensThePRPicker(t *testing.T) {
	m := New(nil, "zed")
	item := repoItem{org: "example", name: "project"}
	updated, cmd := m.Update(cloneDoneMsg{path: "/tmp/project", item: item, loadPulls: true})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("PR clone completion did not load pull requests")
	}
	if got.screen != screenPullRequests {
		t.Fatalf("screen = %v, want PR picker", got.screen)
	}
	if !got.prRepo.cloned {
		t.Fatal("PR repository was not marked cloned")
	}
}

func TestOpenPromptNamesConfiguredEditor(t *testing.T) {
	m := New(nil, "zed")
	updated, _ := m.Update(cloneDoneMsg{path: "/tmp/project", repoName: "project", openEditor: true})
	got := updated.(Model)
	if !strings.Contains(got.View(), "press enter to open in zed") {
		t.Fatalf("open prompt = %q", got.View())
	}
}

func TestWorktreeDirTemplate(t *testing.T) {
	m := New([]Org{{
		Name:                "example",
		CloneDir:            "/src/example",
		WorktreeDirTemplate: "{{.clone_dir}}/.worktrees/{{.repo}}/pr-{{.pr_num}}",
	}}, "zed")
	dir, err := m.worktreeDirFor(repoItem{org: "example", name: "project"}, prItem{number: 42})
	if err != nil {
		t.Fatal(err)
	}
	if want := "/src/example/.worktrees/project/pr-42"; dir != want {
		t.Fatalf("worktree directory = %q, want %q", dir, want)
	}
}

func TestDefaultWorktreeDirTemplate(t *testing.T) {
	m := New([]Org{{Name: "example", CloneDir: "/src/example"}}, "zed")
	dir, err := m.worktreeDirFor(repoItem{org: "example", name: "project"}, prItem{number: 42})
	if err != nil {
		t.Fatal(err)
	}
	if want := "/src/example/project.wt/pr-42"; dir != want {
		t.Fatalf("worktree directory = %q, want %q", dir, want)
	}
}

func TestRefreshErrorDoesNotInterruptClone(t *testing.T) {
	m := New([]Org{{Name: "openshift"}}, "zed")
	m.repoPicker.loading = true
	m.repoPicker.loadingMsg = "Cloning origin…"
	m.repoPicker.pendingFetch = 1

	updated, _ := m.Update(reposErrorMsg{org: "openshift"})
	got := updated.(Model)

	if got.screen != screenRepo {
		t.Fatal("repository refresh error interrupted clone")
	}
	if !got.repoPicker.loading {
		t.Fatal("repository refresh error dismissed clone loading state")
	}
}

func TestCloneProgressCarriageReturnReplacesLastLine(t *testing.T) {
	r := newRepoPicker()
	r.addCloneOutput("Cloning into '/tmp/origin'…", false)
	r.addCloneOutput("Receiving objects: 41%", true)
	r.addCloneOutput("Receiving objects: 42%", true)
	r.addCloneOutput("Receiving objects: 100%, done.", false)

	if len(r.cloneOutput) != 2 {
		t.Fatalf("got %d output lines", len(r.cloneOutput))
	}
	if r.cloneOutput[0] != "Cloning into '/tmp/origin'…" {
		t.Fatalf("first output = %q", r.cloneOutput[0])
	}
	if r.cloneOutput[1] != "Receiving objects: 100%, done." {
		t.Fatalf("progress output = %q", r.cloneOutput[1])
	}
}

func TestCloneWaitsForEnterBeforeOpeningEditor(t *testing.T) {
	m := New(nil, "zed")
	updated, cmd := m.Update(cloneDoneMsg{
		path:       "/tmp/origin",
		repoName:   "origin",
		message:    "cloned to /tmp/origin",
		openEditor: true,
	})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("clone completion opened the editor immediately")
	}
	if got.pendingOpenPath != "/tmp/origin" {
		t.Fatalf("pending path = %q", got.pendingOpenPath)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(Model)
	if cmd == nil {
		t.Fatal("enter did not start opening the editor")
	}
	if got.pendingOpenPath != "" {
		t.Fatal("pending path was not cleared")
	}
}

func TestFilterRanksExactRepositoryNamesBeforeFuzzyMatches(t *testing.T) {
	r := newRepoPicker()
	r.allItems = []repoItem{
		{org: "liouk", name: "origin"},
		{org: "openshift", name: "operator-git-integration"},
		{org: "openshift", name: "origin"},
	}
	r.filter.SetValue("origin")
	r.applyFilter()

	items := r.list.Items()
	if len(items) != 3 {
		t.Fatalf("got %d matches", len(items))
	}
	if items[0].(repoItem).name != "origin" || items[1].(repoItem).name != "origin" {
		t.Fatalf("exact matches were not first: %#v, %#v", items[0], items[1])
	}
	if items[2].(repoItem).name != "operator-git-integration" {
		t.Fatalf("fuzzy match had unexpected position: %#v", items[2])
	}
}

func TestFilterMatchesOwnerAndRepositoryTogether(t *testing.T) {
	r := newRepoPicker()
	r.allItems = []repoItem{
		{org: "liouk", name: "origin"},
		{org: "openshift", name: "origin"},
	}

	for query, want := range map[string]string{
		"liori":  "liouk",
		"opeori": "openshift",
	} {
		r.filter.SetValue(query)
		r.applyFilter()
		items := r.list.Items()
		if len(items) == 0 {
			t.Fatalf("%q returned no matches", query)
		}
		if got := items[0].(repoItem).org; got != want {
			t.Fatalf("%q selected %q, want %q", query, got, want)
		}
	}
}
