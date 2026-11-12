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
    custom_field:
      name: Priority
      default: "P2 - Normal"
      shortcuts:
        0: "P0 - Urgent"
        1: "P1 - High"
        2: "P2 - Normal"
        3: "P3 - Low"
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

Each project may configure one `custom_field`. GitHub supplies its type and
available options dynamically. Single-select, text, number, and date fields are
supported; other types produce an error before adding anything.

After entering a PR link, select the field value on a separate screen. For a
single-select field, `default` sets the initial selection and optional `shortcuts`
map single digits to exact GitHub option names. Use `j`/`k` or arrow keys and
`Enter`, or press a configured digit to accept that option immediately. With no
default, the first option is highlighted. `Esc` or `Ctrl-C` cancels before adding
the PR. Defaults and shortcuts must match existing options.

Text, number, and date fields use an input prompt with the configured default;
dates must use `YYYY-MM-DD`. Shortcuts are only supported for single-select
fields. Batch adds prompt once and apply the chosen value to every PR. `ghproj add
[pr-reference]` also prompts when the project has a custom field. Moving PRs keeps
the existing move behavior without prompting for a field value.

Adding an item and setting its field are separate GitHub operations. If the field
update fails, the PR remains in the project and the command reports the failure.

Requires authenticated `gh`, `jq`, and `gum` for interactive use.

Run the local tests with `python3 -m unittest discover -s tests -v`. They use mocked
GitHub commands and temporary configuration files.
