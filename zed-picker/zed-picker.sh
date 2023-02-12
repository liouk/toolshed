#!/usr/bin/env bash

export _ZO_DATA_DIR="$HOME/.config/toolshed/zed-picker"
mkdir -p "$_ZO_DATA_DIR"

selected=$(
  {
    zoxide query --list 2>/dev/null
    echo "$HOME/redhat"
    fd --type d --no-ignore --max-depth 3 . "$HOME/redhat" 2>/dev/null
    fd --type d --no-ignore --max-depth 1 . "$HOME/redhat/repos"/*//*.wt 2>/dev/null
    fd --type d --no-ignore --max-depth 2 . "$HOME/liouk" 2>/dev/null
    fd --type d --no-ignore --max-depth 1 . "$HOME/Syncthing" 2>/dev/null
    echo "$HOME/.apparatus"
  } | sed 's:/$::' | awk '!seen[$0]++' | fzf --prompt=":: zed :: " --height=40% --reverse
)

if [[ -n "$selected" ]]; then
	# entries can be edited and removed with `zoxide edit/remove`
  zoxide add "$selected"
  zeditor "$(realpath "$selected")"
fi