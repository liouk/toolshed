#!/usr/bin/env bash
set -euo pipefail

config_file="$HOME/.config/toolshed/skratch/config.sh"
if [[ -f "$config_file" ]]; then
  source "$config_file"
fi
: "${SKRATCH_TARGET:?SKRATCH_TARGET must be set in $config_file}"
target="$SKRATCH_TARGET"

tmpfile=$(mktemp /tmp/skratch.XXXXXX.md)
merged_file=""
trap 'rm -f "$tmpfile"; if [[ -n "$merged_file" ]]; then rm -f "$merged_file"; fi' EXIT

printf '# \n' > "$tmpfile"

nvim "+startinsert!" "$tmpfile"

content=$(cat "$tmpfile")
if [[ "$content" == "# " ]] || [[ -z "$content" ]]; then
    echo "Empty note, nothing written."
    exit 0
fi

mkdir -p "$(dirname "$target")"

merged_file=$(mktemp "$(dirname "$target")/.skratch.XXXXXX")
if [[ -f "$target" ]]; then
    cp -p -- "$target" "$merged_file"
fi

{
    printf '%s\n\n' "$content"
    if [[ -f "$target" ]]; then
        cat -- "$target"
    fi
} > "$merged_file"
mv -f -- "$merged_file" "$target"
echo "Note prepended to $target"
