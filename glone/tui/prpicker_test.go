package tui

import "testing"

func TestPullRequestFilterMatchesNumberAndTitle(t *testing.T) {
	p := newPRPicker()
	p.loading = false
	p.allItems = []prItem{
		{number: 12, title: "Improve login", author: "alice"},
		{number: 34, title: "Fix cache", author: "bob"},
	}
	p.filter.SetValue("34")
	p.applyFilter()

	items := p.list.Items()
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if got := items[0].(prItem).number; got != 34 {
		t.Fatalf("PR number = %d, want 34", got)
	}
}
