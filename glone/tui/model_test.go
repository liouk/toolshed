package tui

import (
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
