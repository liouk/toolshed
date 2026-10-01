#!/usr/bin/env zsh

# Replace a Go dependency with a pushed ref from a fork.
usage="usage: go-replace <dependency> <ref> [owner/repo]"

if [[ "${1:-}" == help || "${1:-}" == -h || "${1:-}" == --help ]]; then
  print -r -- "$usage"
  exit
elif (( $# < 2 || $# > 3 )); then
  print -u2 -r -- "$usage"
  exit 2
fi

query="$1"
ref="$2"
fork="$3"
typeset -a exact_matches name_matches substring_matches matches

deps="$(go list -mod=mod -m -f '{{if not .Main}}{{.Path}}{{end}}' all)" || exit
while IFS= read -r dep; do
  if [[ "$dep" == "$query" || "$dep" == "github.com/$query" ]]; then
    exact_matches+=("$dep")
  elif [[ "${dep:t}" == "$query" ]]; then
    name_matches+=("$dep")
  elif [[ "$dep" == *"$query"* ]]; then
    substring_matches+=("$dep")
  fi
done <<< "$deps"

if (( ${#exact_matches} )); then
  matches=("${exact_matches[@]}")
elif (( ${#name_matches} )); then
  matches=("${name_matches[@]}")
else
  matches=("${substring_matches[@]}")
fi

case ${#matches} in
  0)
    print -u2 "go-replace: no dependency matches '$query'"
    exit 1
    ;;
  1) upstream="${matches[1]}" ;;
  *)
    print -u2 "go-replace: multiple dependencies match '$query':"
    printf '  %s\n' "${matches[@]}" >&2
    exit 1
    ;;
esac

if [[ -z "$fork" ]]; then
  if [[ "$upstream" != github.com/*/* ]]; then
    print -u2 "go-replace: cannot derive a GitHub fork from '$upstream'"
    exit 1
  fi
  fork="liouk/${${upstream#github.com/}#*/}"
fi
fork="github.com/${fork#github.com/}"

version="$(go list -mod=mod -m -f '{{.Version}}' "${fork}@${ref}")" || exit
go mod edit -replace="${upstream}=${fork}@${version}" || exit
print -r -- "replace $upstream => $fork $version"
