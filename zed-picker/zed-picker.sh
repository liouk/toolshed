#!/usr/bin/env bash

config_dir="${ZED_PICKER_CONFIG_DIR:-$HOME/.config/toolshed/zed-picker}"
mkdir -p "$config_dir"
export _ZO_DATA_DIR="$config_dir"

config_file="${ZED_PICKER_CONFIG_FILE:-$config_dir/config.sh}"

ZED_PICKER_FIXED_PATHS=()
ZED_PICKER_SEARCH_PATHS=()
ZED_PICKER_WORKTREE_GLOBS=()

if [[ -r "$config_file" ]]; then
	source "$config_file"
fi

selected=$(
  {
    zoxide query --list 2>/dev/null

	printf '%s\n' "${ZED_PICKER_FIXED_PATHS[@]}"

	for search in "${ZED_PICKER_SEARCH_PATHS[@]}"; do
		root="${search%:*}"
		max_depth="${search##*:}"
		fd --type d --no-ignore --max-depth "$max_depth" . "$root" 2>/dev/null
	done

	for worktree_glob in "${ZED_PICKER_WORKTREE_GLOBS[@]}"; do
		fd --type d --no-ignore --max-depth 1 . "$worktree_glob" 2>/dev/null
	done
  } | sed 's:/$::' | awk '!seen[$0]++' | fzf --prompt=":: zed :: " --height=40% --reverse
)

if [[ -n "$selected" ]]; then
	# entries can be edited and removed with `zoxide edit/remove`
  zoxide add "$selected"
  zeditor "$(realpath "$selected")"
fi
