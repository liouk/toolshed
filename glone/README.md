# glone

A TUI for browsing, cloning, and opening GitHub repos from your orgs.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

```
go install github.com/liouk/toolshed/glone@latest
```

Or build from source:

```
cd glone && go build -o glone .
```

Requires:
- Go 1.25+
- [gh CLI](https://cli.github.com) (authenticated)

## GitHub authentication

`glone` uses the GitHub CLI for repository listing, cloning, forking, and
browsing. Clones use SSH remotes, so ensure your GitHub SSH key is configured.
Authenticate `gh` once with a GitHub personal access token; the
token is stored by `gh` and does not need to be exported globally:

```bash
printf 'GitHub PAT: ' >&2
IFS= read -r -s GH_PAT
printf '\n' >&2
printf '%s' "$GH_PAT" | gh auth login --hostname github.com --with-token
unset GH_PAT
```

For a classic personal access token, grant `repo` and `read:org`; add the
`project` scope if you use the same token with `ghproj` or `jira2gh`.

Verify the stored credentials with:

```bash
env -u GH_TOKEN -u GITHUB_TOKEN gh auth status
```

Avoid exporting an unrelated `GH_TOKEN` or `GITHUB_TOKEN`: `gh` gives those
environment variables precedence over its stored credentials.

## Config

Create `~/.config/toolshed/glone/config.yaml`:

```yaml
editor: code                             # optional; auto-opens repos after clone
orgs:
  - name: my-company
    clone_dir: ~/src/my-company          # repos clone to ~/src/my-company/<repo>
    exclude:                             # hide repos from the list
      - .github
      - old-project
  - name: my-username
    clone_dir: ~/src/personal            # original repos → ~/src/personal/<repo>
    fork_clone_dirs:                     # forks routed by parent org:
      my-company: ~/src/my-company-forks #   company forks → ~/src/my-company-forks/<repo>
                                         #   other forks → ~/src/personal/<repo> (default)
```

| Field | Required | Description |
|-------|----------|-------------|
| `editor` | no | Editor binary to open repos in after clone |
| `orgs[].name` | yes | GitHub org or username |
| `orgs[].clone_dir` | yes | Directory to clone repos into (`<clone_dir>/<repo>`) |
| `orgs[].fork_clone_dirs` | no | Map of parent org → clone dir for forked repos |
| `orgs[].exclude` | no | List of repo names to hide from the list |

## Usage

```
glone
```

If multiple orgs are configured, you'll first pick one (press `1`-`9`/`0` to quick-select). Then browse repos with fuzzy filtering.

### Keybindings

| Key | Action |
|-----|--------|
| *type* | Fuzzy filter repos |
| `enter` | Clone repo (or open if already cloned) |
| `ctrl+s` | Shallow clone (`--depth 1`) |
| `ctrl+f` | Fork repo, clone the fork, and open |
| `ctrl+o` | Open in browser |
| `esc` / `ctrl+c` | Quit |

Already-cloned repos are marked with ✓ and `enter` opens them in the configured `editor`. After cloning, the repo auto-opens in the editor if configured; otherwise the path is printed to stdout.
While a clone runs, glone shows the most recent 24 lines of clone output below its progress indicator.

`ctrl+f` forks the selected repo under your GitHub user, clones the fork (using `fork_clone_dirs` to pick the destination), and opens it. Requires a matching org entry in your config for your GitHub username.

### Shell integration

`glone` prints the repo path to stdout on success, so you can:

```bash
cd "$(glone)"
```
