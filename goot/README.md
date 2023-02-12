# goot

A minimal TUI for quickly adding tasks to Google Tasks, and a CLI for querying completed tasks.

## Setup

1. Create a Google Cloud project and enable the [Tasks API](https://console.cloud.google.com/apis/library/tasks.googleapis.com)
2. Create an OAuth 2.0 credential (Application type: **Desktop app**)
3. Download the credentials JSON and place it at `~/.config/goot/credentials.json`

On first run, `goot` will open your browser for OAuth consent. The token is cached at `~/.config/goot/token.json` for subsequent runs.

## Install

```
go install github.com/liouk/toolshed/goot@latest
```

Or build from source:

```
cd goot && go build -o goot .
```

## Usage

```
goot
goot list <list-name>
goot done <range>
goot help
```

- No arguments: starts at the list selection screen with fuzzy filtering
- `goot list <name>`: skips selection and jumps straight to task creation (case-insensitive match; falls back to selection if no match)

### Completed tasks

`goot done` fetches completed tasks across all task lists and prints them as markdown, grouped by list.

**Shortcuts:**

```
goot done today
goot done yesterday
goot done this-week
goot done last-week
goot done this-month
goot done last-month
goot done this-quarter
goot done last-quarter
goot done this-year
goot done last-year
```

**Explicit date range:**

```
goot done --from 2026-01-01 --to 2026-03-31
```

The output is both human-readable and suitable for piping into an LLM:

```
goot done last-quarter | claude -p "summarize what I worked on"
```

## Configuration

Optional. Create `~/.config/goot/config.json` to customize behavior:

```json
{
  "hidden_lists_by_id": ["list-id-1", "list-id-2"]
}
```

| Key | Description |
|-----|-------------|
| `hidden_lists_by_id` | List IDs to hide from the picker (use `x` in the picker to hide lists interactively) |

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down (picker screen) |
| `1`-`9` | Select list by number (picker screen) |
| `/` | Filter lists (picker screen) |
| `x` | Hide list from picker (picker screen) |
| `tab` / `shift+tab` | Navigate between form fields |
| `enter` | Select list / submit task |
| `backspace` | Go back to list selection (creator screen, empty title) |
| `esc` | Quit with confirmation (creator screen) |
| `ctrl+c` | Quit immediately |

## Task fields

| Field | Required | Format |
|-------|----------|--------|
| Title | yes | free text |
| Notes | no | free text |
| Due | no | `YYYY-MM-DD` (auto-fills today on first focus) |

Due dates are interpreted as CET/CEST.
