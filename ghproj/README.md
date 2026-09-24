# ghproj

```
ghproj — manage PRs in a GitHub Project (v2) via `gh`

Usage:
  ghproj        interactive mode via `gum`
  ghproj list
  ghproj add    [pr-reference]
  ghproj remove <pr-reference>
  ghproj clear  [closed|merged|not-open|all]
  ghproj config <owner> <number> [view-id] [secondary-project-number]
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

GitHub authentication:

`ghproj` uses the GitHub CLI for all GitHub operations. Authenticate `gh`
once with a personal access token; the token is stored by `gh` and does not
need to be exported globally:

```bash
printf 'GitHub PAT: ' >&2
IFS= read -r -s GH_PAT
printf '\n' >&2
printf '%s' "$GH_PAT" | gh auth login --hostname github.com --with-token
unset GH_PAT
```

For a classic personal access token, grant the `project` scope, plus `repo`
and `read:org` if the project contains private or organization repositories.
Check the stored credentials with:

```bash
env -u GH_TOKEN -u GITHUB_TOKEN gh auth status
```

Avoid exporting an unrelated `GH_TOKEN` or `GITHUB_TOKEN`: `gh` gives those
environment variables precedence over its stored credentials.

Config file (used when the env vars above aren't set):
  ~/.config/toolshed/ghproj/config.yaml
    owner: octocat
    project: 5
    view_id: 2688867
    secondary_project: 6
  Write it with: ghproj config <owner> <number> [view-id] [secondary-project-number]

When launched interactively without a config file or project environment
variables, ghproj prompts for the owner and project number, plus an optional
pull-request view ID, then saves the values to the config file.

In the interactive menu, press `A` to paste multiple PR links into the current
project with a multiline text box. Links may be separated by commas, spaces, or
newlines. Press `t` or `T` to add one or multiple PRs directly to the configured
secondary project.

Press `m` to move selected PRs to another project. Set `secondary_project` in
the config file to use that project without being prompted; its name is shown in
the menu when it can be resolved.

In the secondary-project section, `R` removes selected PRs and `X` clears all
PRs from that project.

Requires:
  gh: authenticated, with `project` scope
  jq: for parsing gh responses
  gum: for the interactive prompts and menu
  wl-paste: for checking clipboard for an already pasted PR string
```
