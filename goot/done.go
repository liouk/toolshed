package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// DateRange represents a date interval [From, To].
type DateRange struct {
	From time.Time // start of day (00:00:00)
	To   time.Time // end of day (23:59:59)
}

// CompletedTask holds a completed task with its completion date.
type CompletedTask struct {
	Title     string
	Notes     string
	Completed string // YYYY-MM-DD
}

// parseDateRange turns a shortcut name or --from/--to pair into a DateRange.
func parseDateRange(args []string) (DateRange, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if len(args) == 1 {
		return resolveShortcut(args[0], today)
	}

	// Expect --from DATE --to DATE (any order).
	var from, to string
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--from":
			from = args[i+1]
			i++
		case "--to":
			to = args[i+1]
			i++
		}
	}
	if from == "" || to == "" {
		return DateRange{}, fmt.Errorf("usage: goot done <shortcut> | goot done --from YYYY-MM-DD --to YYYY-MM-DD\n\nshortcuts: today, yesterday, this-week, last-week, this-month, last-month, this-quarter, last-quarter, this-year, last-year")
	}

	f, err := time.ParseInLocation("2006-01-02", from, now.Location())
	if err != nil {
		return DateRange{}, fmt.Errorf("parse --from: %w", err)
	}
	t, err := time.ParseInLocation("2006-01-02", to, now.Location())
	if err != nil {
		return DateRange{}, fmt.Errorf("parse --to: %w", err)
	}
	return DateRange{From: f, To: endOfDay(t)}, nil
}

func resolveShortcut(name string, today time.Time) (DateRange, error) {
	year, month, _ := today.Date()
	loc := today.Location()

	switch name {
	case "today":
		return DateRange{From: today, To: endOfDay(today)}, nil

	case "yesterday":
		y := today.AddDate(0, 0, -1)
		return DateRange{From: y, To: endOfDay(y)}, nil

	case "this-week":
		// Week starts on Monday.
		wd := (int(today.Weekday()) + 6) % 7 // Monday=0
		mon := today.AddDate(0, 0, -wd)
		return DateRange{From: mon, To: endOfDay(today)}, nil

	case "last-week":
		wd := (int(today.Weekday()) + 6) % 7
		mon := today.AddDate(0, 0, -wd-7)
		sun := mon.AddDate(0, 0, 6)
		return DateRange{From: mon, To: endOfDay(sun)}, nil

	case "this-month":
		first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		return DateRange{From: first, To: endOfDay(today)}, nil

	case "last-month":
		first := time.Date(year, month-1, 1, 0, 0, 0, 0, loc)
		last := time.Date(year, month, 1, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
		return DateRange{From: first, To: endOfDay(last)}, nil

	case "this-quarter":
		q := (month - 1) / 3 * 3
		first := time.Date(year, q+1, 1, 0, 0, 0, 0, loc)
		return DateRange{From: first, To: endOfDay(today)}, nil

	case "last-quarter":
		q := (month - 1) / 3 * 3
		first := time.Date(year, q-2, 1, 0, 0, 0, 0, loc)
		last := time.Date(year, q+1, 1, 0, 0, 0, 0, loc).AddDate(0, 0, -1)
		return DateRange{From: first, To: endOfDay(last)}, nil

	case "this-year":
		first := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		return DateRange{From: first, To: endOfDay(today)}, nil

	case "last-year":
		first := time.Date(year-1, 1, 1, 0, 0, 0, 0, loc)
		last := time.Date(year-1, 12, 31, 0, 0, 0, 0, loc)
		return DateRange{From: first, To: endOfDay(last)}, nil

	default:
		return DateRange{}, fmt.Errorf("unknown shortcut %q\n\navailable: today, yesterday, this-week, last-week, this-month, last-month, this-quarter, last-quarter, this-year, last-year", name)
	}
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
}

type listResult struct {
	Title string
	Tasks []CompletedTask
}

// runDone fetches completed tasks across all lists and prints them as markdown.
func runDone(ctx context.Context, client *Client, args []string) error {
	dr, err := parseDateRange(args)
	if err != nil {
		return err
	}

	lists, err := client.Lists(ctx)
	if err != nil {
		return err
	}

	var results []listResult
	for _, l := range lists {
		tasks, err := client.CompletedTasks(ctx, l.ID, dr)
		if err != nil {
			return fmt.Errorf("list %q: %w", l.Title, err)
		}
		if len(tasks) > 0 {
			results = append(results, listResult{Title: l.Title, Tasks: tasks})
		}
	}

	printDone(os.Stdout, dr, results)
	return nil
}

func printDone(w *os.File, dr DateRange, results []listResult) {
	fmt.Fprintf(w, "# Completed Tasks: %s to %s\n\n", dr.From.Format("2006-01-02"), dr.To.Format("2006-01-02"))

	if len(results) == 0 {
		fmt.Fprintln(w, "No completed tasks in this period.")
		return
	}

	for _, r := range results {
		fmt.Fprintf(w, "## %s\n\n", r.Title)
		for _, t := range r.Tasks {
			fmt.Fprintf(w, "- %s (completed: %s)\n", t.Title, t.Completed)
			if t.Notes != "" {
				for _, line := range strings.Split(t.Notes, "\n") {
					fmt.Fprintf(w, "  %s\n", line)
				}
			}
		}
		fmt.Fprintln(w)
	}
}
