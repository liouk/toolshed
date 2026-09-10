# ghproj

```
ghproj — manage PRs in a GitHub Project (v2) via `gh`

Usage:
  ghproj        interactive mode via `gum`
  ghproj list
  ghproj add    [pr-reference]
  ghproj remove <pr-reference>
  ghproj clear  [closed|merged|not-open|all]
  ghproj config <owner> <number> [view-id]
  ghproj help

pr-reference (any of):
  https://github.com/owner/repo/pull/372
  owner/repo#372
  owner/repo 372
  owner repo 372
  omit it on `add` to be prompted interactively

Commands:
  list           show every PR item currently in the project
  add            add a PR to the project (prompts if none given)
  remove         remove one PR from the project
  clear [mode]   bulk-remove PR items by state (default: not-open):
                   closed     state == CLOSED (closed without merging)
                   merged     state == MERGED
                   not-open   state != OPEN (closed or merged)
                   all        every PR item, regardless of state
  config         save owner/number/view-id to the config file (see below)
  help           show this message

Env (override the config file):
  PROJECT_OWNER   user or org login that owns the project (e.g. "octocat")
  PROJECT_NUMBER  project number, from its URL (…/projects/<N>)
  GHPROJ_BROWSER   browser executable for v/p shortcuts (default: firefox)

Config file (used when the env vars above aren't set):
  ~/.config/toolshed/ghproj/config.yaml
    owner: octocat
    number: 5
    view_id: 2688867
  Write it with: ghproj config <owner> <number> [view-id]

When launched interactively without a config file or project environment
variables, ghproj prompts for the owner and project number, plus an optional
pull-request view ID, then saves the values to the config file.

Requires:
  gh: authenticated, with `project` scope
  jq: for parsing gh responses
  gum: for the interactive prompts and menu
  wl-paste: for checking clipboard for an already pasted PR string
```
