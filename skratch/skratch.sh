#!/usr/bin/env bash
set -euo pipefail

config_file="$HOME/.config/toolshed/skratch/config"
if [[ -f "$config_file" ]]; then
  source "$config_file"
fi
target="${SKRATCH_TARGET:-$HOME/Syncthing/notes/opa.md}"

tmpfile=$(mktemp /tmp/skratch.XXXXXX.md)
trap 'rm -f "$tmpfile"' EXIT

printf '# \n' > "$tmpfile"

nvim "+startinsert!" "$tmpfile"

content=$(cat "$tmpfile")
if [[ "$content" == "# " ]] || [[ -z "$content" ]]; then
    echo "Empty note, nothing written."
    exit 0
fi

mkdir -p "$(dirname "$target")"

if [[ -s "$target" ]]; then
    printf '\n\n' >> "$target"
fi

printf '%s\n' "$content" >> "$target"
echo "Note appended to $target"
