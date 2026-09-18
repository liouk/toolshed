#!/usr/bin/env bash

config_dir="${ZED_PICKER_CONFIG_DIR:-$HOME/.config/toolshed/zed-picker}"
mkdir -p "$config_dir"
export _ZO_DATA_DIR="$config_dir"

config_file="${ZED_PICKER_CONFIG_FILE:-$config_dir/config.sh}"

ZED_PICKER_PATHS=()

if [[ -r "$config_file" ]]; then
	source "$config_file"
fi

selected=$(
  {
    zoxide query --list 2>/dev/null

	for ((i = 0; i < ${#ZED_PICKER_PATHS[@]}; i++)); do
		entry="${ZED_PICKER_PATHS[i]}"
		value="$entry"
		max_depth=""

		# A trailing :N limits either a search or a glob. Colons elsewhere
		# are treated as part of the path.
		if [[ "$value" =~ ^(.*):([0-9]+)$ ]]; then
			value="${BASH_REMATCH[1]}"
			max_depth="${BASH_REMATCH[2]}"
		fi

		if [[ "$value" == *"*"* || "$value" == *"?"* || "$value" == *"["* ]]; then
			# Search from the part before the first wildcard, while matching
			# against the complete path. This keeps the glob constrained to
			# its configured tree.
			glob_prefix="$value"
			for ((j = 0; j < ${#value}; j++)); do
				character="${value:j:1}"
				if [[ "$character" == "*" || "$character" == "?" || "$character" == "[" ]]; then
					glob_prefix="${value:0:j}"
					break
				fi
			done
			glob_root="${glob_prefix%/*}"
			[[ -n "$glob_root" ]] || glob_root="."

			fd_args=(--type d --no-ignore --glob --full-path)
			[[ -n "$max_depth" ]] && fd_args+=(--max-depth "$max_depth")
			fd "${fd_args[@]}" "$value" "$glob_root" 2>/dev/null
		elif [[ -n "$max_depth" ]]; then
			fd --type d --no-ignore --max-depth "$max_depth" . "$value" 2>/dev/null
		else
			printf '%s\n' "$value"
		fi
	done
	  } |
		sed 's:/$::' |
		awk -v home="$HOME" '{
			display = $0
			sub("^" home, "~", display)
			print display "\t" $0
		}' |
		awk -F '\t' '!seen[$2]++' |
		fzf --delimiter=$'\t' --with-nth=1 --accept-nth=2 \
			--prompt=":: zed :: " --height=40% --reverse
)

if [[ -n "$selected" ]]; then
	# entries can be edited and removed with `zoxide edit/remove`
  zoxide add "$selected"
  zeditor "$(realpath "$selected")"
fi
