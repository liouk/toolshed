package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"google.golang.org/api/option"
	tasks "google.golang.org/api/tasks/v1"
)

var cetLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
	return loc
}()

type TaskList struct {
	ID    string
	Title string
}

type Task struct {
	Title string
	Notes string
	Due   string // YYYY-MM-DD or empty
}

type Client struct {
	svc *tasks.Service
}

func NewClient(ctx context.Context, httpClient *http.Client) (*Client, error) {
	svc, err := tasks.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create tasks service: %w", err)
	}
	return &Client{svc: svc}, nil
}

func (c *Client) Lists(ctx context.Context) ([]TaskList, error) {
	resp, err := c.svc.Tasklists.List().Context(ctx).MaxResults(100).Do()
	if err != nil {
		return nil, fmt.Errorf("list task lists: %w", err)
	}

	lists := make([]TaskList, len(resp.Items))
	for i, item := range resp.Items {
		lists[i] = TaskList{ID: item.Id, Title: item.Title}
	}
	return lists, nil
}

func (c *Client) Tasks(ctx context.Context, listID string) ([]Task, error) {
	resp, err := c.svc.Tasks.List(listID).Context(ctx).MaxResults(100).ShowCompleted(false).Do()
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	result := make([]Task, len(resp.Items))
	for i, item := range resp.Items {
		var due string
		if item.Due != "" {
			if t, err := time.Parse(time.RFC3339, item.Due); err == nil {
				due = t.Format("2006-01-02")
			}
		}
		result[i] = Task{Title: item.Title, Notes: item.Notes, Due: due}
	}
	return result, nil
}

func (c *Client) CompletedTasks(ctx context.Context, listID string, dr DateRange) ([]CompletedTask, error) {
	minRFC := dr.From.Format(time.RFC3339)
	maxRFC := dr.To.Format(time.RFC3339)

	var all []CompletedTask
	call := c.svc.Tasks.List(listID).Context(ctx).
		MaxResults(100).
		ShowCompleted(true).
		ShowHidden(true).
		CompletedMin(minRFC).
		CompletedMax(maxRFC)

	for {
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list completed tasks: %w", err)
		}

		for _, item := range resp.Items {
			if item.Completed == nil || *item.Completed == "" {
				continue
			}
			var completed string
			if t, err := time.Parse(time.RFC3339, *item.Completed); err == nil {
				completed = t.Format("2006-01-02")
			}
			all = append(all, CompletedTask{
				Title:     item.Title,
				Notes:     item.Notes,
				Completed: completed,
			})
		}

		if resp.NextPageToken == "" {
			break
		}
		call = call.PageToken(resp.NextPageToken)
	}

	return all, nil
}

var githubPRRe = regexp.MustCompile(`https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)\S*`)
var githubRepoRe = regexp.MustCompile(`https?://github\.com/([^/\s]+)/([^/\s]+?)(?:\.git|/)?(?:\s|$)`)
var jiraRe = regexp.MustCompile(`https?://[^/\s]+/browse/([A-Z][A-Z0-9]+-\d+)\S*`)

func processGitHubURLs(task Task) Task {
	var links []string

	task.Title = githubPRRe.ReplaceAllStringFunc(task.Title, func(match string) string {
		m := githubPRRe.FindStringSubmatch(match)
		links = append(links, match)
		return m[1] + "/" + m[2] + "#" + m[3]
	})

	task.Title = jiraRe.ReplaceAllStringFunc(task.Title, func(match string) string {
		m := jiraRe.FindStringSubmatch(match)
		links = append(links, match)
		return m[1]
	})

	task.Title = githubRepoRe.ReplaceAllStringFunc(task.Title, func(match string) string {
		m := githubRepoRe.FindStringSubmatch(match)
		url := strings.TrimRight(match, " \t\n")
		links = append(links, url)
		trailing := match[len(url):]
		return m[1] + "/" + m[2] + trailing
	})

	if len(links) > 0 {
		var linkLines []string
		for _, l := range links {
			linkLines = append(linkLines, "Link: "+l)
		}
		suffix := strings.Join(linkLines, "\n")
		if task.Notes != "" {
			task.Notes += "\n" + suffix
		} else {
			task.Notes = suffix
		}
	}

	return task
}

func (c *Client) CreateTask(ctx context.Context, listID string, task Task) error {
	task = processGitHubURLs(task)
	t := &tasks.Task{
		Title: task.Title,
		Notes: task.Notes,
	}

	if task.Due != "" {
		parsed, err := time.ParseInLocation("2006-01-02", task.Due, cetLocation)
		if err != nil {
			return fmt.Errorf("parse due date: %w", err)
		}
		// Use 9am to avoid UTC conversion shifting the date back a day.
		morning := parsed.Add(9 * time.Hour)
		t.Due = morning.Format(time.RFC3339)
	}

	_, err := c.svc.Tasks.Insert(listID, t).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return nil
}
