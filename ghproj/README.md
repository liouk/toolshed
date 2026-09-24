# ghproj

`ghproj` manages pull requests in one or more GitHub Projects (v2).

```text
ghproj
ghproj list
ghproj add [pr-reference]
ghproj config <owner> <project> [view-url]
ghproj config add <project> [view-url]
ghproj config remove <project>
```

Configure projects in `~/.config/toolshed/ghproj/config.yaml`:

```yaml
owner: my-org
projects:
  - project: 5
    view_url: https://github.com/pulls/SSC_kgDOACmseA
  - project: 6
```

The interactive UI displays the configured projects at the top. Press `Tab` or
`Shift+Tab` to choose one; all menu actions apply to that selected project.

| Key | Action |
| --- | --- |
| `a` / `A` | Add one PR / paste multiple PR links |
| `c` | Clear closed and merged PRs |
| `r` | Select PRs to remove |
| `x` | Clear all PRs |
| `l` | List PRs |
| `v` / `p` | Open the PR view / project page |
| `m` | Move selected PRs to another configured project |
| `q`, `esc`, `ctrl-c` | Quit |

Multiple pasted PR links may be comma-, space-, or newline-separated. Moving a
PR adds it to the destination before removing it from the selected source.

Requires authenticated `gh`, `jq`, and `gum` for interactive use.
