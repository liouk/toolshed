#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/toolshed/ghproj/config.yaml"
PROJECT_OWNER="${PROJECT_OWNER:-}"
declare -a PROJECTS=() VIEWS=() TITLES=()

load_config() {
  [ -f "$CONFIG_FILE" ] || return 0
  : "${PROJECT_OWNER:=$(sed -n 's/^owner:[[:space:]]*//p' "$CONFIG_FILE" | head -1)}"
  while IFS='|' read -r p v; do
    [ -z "$p" ] || { PROJECTS+=("$p"); VIEWS+=("$v"); TITLES+=(""); }
  done < <(awk '/^[[:space:]]*-[[:space:]]*project:/{if(p!="")print p"|"v;sub(/.*project:[[:space:]]*/,"");p=$0;v="";next}/^[[:space:]]+view_url:/{sub(/.*view_url:[[:space:]]*/,"");v=$0}END{if(p!="")print p"|"v}' "$CONFIG_FILE")
  if [ "${#PROJECTS[@]}" -eq 0 ]; then
    p=$(sed -n 's/^project:[[:space:]]*//p' "$CONFIG_FILE"|head -1); v=$(sed -n 's/^view_url:[[:space:]]*//p' "$CONFIG_FILE"|head -1)
    [ -z "$p" ] || { PROJECTS+=("$p"); VIEWS+=("$v"); TITLES+=(""); }
    p=$(sed -n 's/^secondary_project:[[:space:]]*//p' "$CONFIG_FILE"|head -1)
    [ -z "$p" ] || { PROJECTS+=("$p"); VIEWS+=(""); TITLES+=(""); }
  fi
}
save_config() {
  local i; mkdir -p "$(dirname "$CONFIG_FILE")"
  { printf 'owner: %s\nprojects:\n' "$PROJECT_OWNER"; for i in "${!PROJECTS[@]}"; do
      printf '  - project: %s\n' "${PROJECTS[i]}"; [ -z "${VIEWS[i]}" ] || printf '    view_url: %s\n' "${VIEWS[i]}"
    done; } > "$CONFIG_FILE"
}
number() { [[ $1 =~ ^[0-9]+$ ]]; }
title() { local i=$1; [ -n "${TITLES[i]}" ] || TITLES[i]=$(gh project view "${PROJECTS[i]}" --owner "$PROJECT_OWNER" --format json -q .title 2>/dev/null || printf 'Project #%s' "${PROJECTS[i]}"); printf '%s' "${TITLES[i]}"; }
items() { gh project item-list "$1" --owner "$PROJECT_OWNER" --format json --limit 200; }
parse_pr() {
  if [[ $# = 1 && $1 =~ ^(https?://)?github\.com/([^/]+/[^/]+)/pull/([0-9]+)/?$ ]]; then REPO="${BASH_REMATCH[2]}"; NUM="${BASH_REMATCH[3]}"
  elif [[ $# = 1 && $1 =~ ^([^/]+/[^/]+)#([0-9]+)$ ]]; then REPO="${BASH_REMATCH[1]}"; NUM="${BASH_REMATCH[2]}"
  elif [ $# = 2 ]; then REPO=$1; NUM=$2
  elif [ $# = 3 ]; then REPO="$1/$2"; NUM=$3
  else return 1; fi
}
add_one() {
  local p=$1 n=$2 ref; shift 2
  if [ $# = 0 ]; then ref=$(gum input --header "PR to add to \"$n\"" --prompt '❯ ' --placeholder 'owner/repo#123') || return; read -ra refs <<< "$ref"; parse_pr "${refs[@]}" || return 1
  else parse_pr "$@" || return 1; fi
  gh project item-add "$p" --owner "$PROJECT_OWNER" --url "https://github.com/$REPO/pull/$NUM"
}
add_many() {
  local p=$1 n=$2 ref text; shift 2; text=$(gum write --header "PRs to add to \"$n\"" --placeholder 'Paste PR links, separated by commas, spaces, or newlines') || return
  while IFS= read -r ref; do parse_pr "$ref" && gh project item-add "$p" --owner "$PROJECT_OWNER" --url "https://github.com/$REPO/pull/$NUM"; done < <(printf '%s' "$text"|tr ',[:space:]' '\n'|sed '/^$/d')
}
select_prs() {
  local p=$1 n=$2 row id repo num pr url i selection; local -a rows=() choices=(); declare -A seen=()
  IDS=(); URLS=(); mapfile -t rows < <(items "$p"|jq -r '.items[]|select(.content.type=="PullRequest")|[.id,.content.repository,.content.number,.content.title,.content.url]|@tsv')
  [ "${#rows[@]}" -gt 0 ] || return 1
  for i in "${!rows[@]}"; do IFS=$'\t' read -r id repo num pr url <<< "${rows[i]}"; choices+=("[$i] $repo #$num  $pr"); done
  selection=$(printf '%s\n' "${choices[@]}"|gum filter --no-limit --header "PRs in \"$n\"" --height 15 --reverse) || return
  while IFS= read -r row; do [[ $row =~ ^\[([0-9]+)\] ]] || continue; i="${BASH_REMATCH[1]}"; IFS=$'\t' read -r id repo num pr url <<< "${rows[i]}"; [ -n "${seen[$id]+x}" ] || { IDS+=("$id"); URLS+=("$url"); seen[$id]=1; }; done <<< "$selection"
  [ "${#IDS[@]}" -gt 0 ]
}
remove_ids() { local p=$1 id; shift; for id in "$@"; do gh project item-delete "$p" --owner "$PROJECT_OWNER" --id "$id"; done; }
clear() {
  local p=$1 n=$2 mode=$3 id repo num pr url state; local -a ids=()
  while IFS=$'\t' read -r id repo num pr url; do state=$(gh pr view "$url" --json state,mergedAt -q 'if .mergedAt == null then .state else "MERGED" end' 2>/dev/null||echo UNKNOWN); [ "$mode" = all ] || [ "$state" != OPEN ] || continue; printf '  %s  %s #%s  %s\n' "$state" "$repo" "$num" "$pr"; ids+=("$id"); done < <(items "$p"|jq -r '.items[]|select(.content.type=="PullRequest")|[.id,.content.repository,.content.number,.content.title,.content.url]|@tsv')
  [ "${#ids[@]}" -gt 0 ] && gum confirm "Clear these PRs from \"$n\"?" && remove_ids "$p" "${ids[@]}"
}
destination() {
  local source=$1 i choice; local -a choices=()
  for i in "${!PROJECTS[@]}"; do [ "$i" = "$source" ] || choices+=("[$i] $(title "$i")  (#${PROJECTS[i]})"); done
  [ "${#choices[@]}" -gt 0 ] || return 1; choice=$(printf '%s\n' "${choices[@]}"|gum filter --header 'Move PRs to') || return
  [[ $choice =~ ^\[([0-9]+)\] ]] && DEST="${BASH_REMATCH[1]}"
}
move() {
  local source=$1 p=$2 n=$3 i; select_prs "$p" "$n" && destination "$source" || return
  for i in "${!IDS[@]}"; do gh project item-add "${PROJECTS[DEST]}" --owner "$PROJECT_OWNER" --url "${URLS[i]}" && gh project item-delete "$p" --owner "$PROJECT_OWNER" --id "${IDS[i]}"; done
}
cards() {
  local current=$1 i border title_style body_style name label padding number_line
  local top='' name_row='' number_row='' bottom=''
  for i in "${!PROJECTS[@]}"; do
    name=$(title "$i")
    if [ "$i" = "$current" ]; then
      border=$'\033[38;5;215m'
      title_style=$'\033[4;38;5;215m'
      body_style=$'\033[0m'
      label="${name:0:28}"
    else
      border=$'\033[38;5;245m'
      title_style=$'\033[38;5;245m'
      body_style=$'\033[38;5;245m'
      label="${name:0:28}"
    fi
    printf -v padding '%*s' $((28 - ${#label})) ''
    printf -v number_line ' Project #%-19s ' "${PROJECTS[i]}"
    top+="  ${border}╭──────────────────────────────╮"$'\033[0m'
    name_row+="  ${border}│"$'\033[0m '"${title_style}${label}"$'\033[0m'"${padding} ${border}│"$'\033[0m'
    number_row+="  ${border}│"$'\033[0m'"${body_style}${number_line}${border}│"$'\033[0m'
    bottom+="  ${border}╰──────────────────────────────╯"$'\033[0m'
  done
  printf '%s\n%s\n%s\n%s\n' "$top" "$name_row" "$number_row" "$bottom"
}
interactive() {
  local current=0 selected=0 key rest action p n v i; local -a menu=('a  Add a PR' 'A  Add multiple PRs' 'c  Clear closed and merged PRs' 'r  Choose PR(s) to remove' 'x  Clear all PRs' 'l  List PRs' 'v  Open PR view' 'p  Open project view' 'm  Move selected PRs to another project')
  while :; do
    printf '\033[H\033[2JManage PRs\n\n  \033[38;5;241mTab / Shift-Tab selects a project\033[0m\n\n'; cards "$current"; printf '\n'
    for i in "${!menu[@]}"; do [ "$i" = "$selected" ] && printf '\033[38;5;212m❯ %s\033[0m\n' "${menu[i]}" || printf '  %s\n' "${menu[i]}"; case "$i" in 1|4|7) printf '\n';; esac; done
    printf '\n\033[38;5;241m↑/↓ or j/k navigate · enter select · tab switch project · q/esc/ctrl-c quit\033[0m\n'; IFS= read -rsn1 key || return
    case "$key" in
      $'\t') current=$(((current+1)%${#PROJECTS[@]}));;
      $'\e') IFS= read -rsn2 rest || return; case "$rest" in '[A') [ "$selected" -gt 0 ]&&selected=$((selected-1));; '[B') [ "$selected" -lt $((${#menu[@]}-1)) ]&&selected=$((selected+1));; '[Z') current=$(((current-1+${#PROJECTS[@]})%${#PROJECTS[@]}));; *) return;; esac;;
      k) [ "$selected" -gt 0 ]&&selected=$((selected-1));; j) [ "$selected" -lt $((${#menu[@]}-1)) ]&&selected=$((selected+1));;
      q|Q) return;; a|A|c|C|r|R|x|X|l|L|v|V|p|P|m|M) action=$key; break;; ""|$'\n'|$'\r') action="${menu[selected]:0:1}"; break;;
    esac
  done
  p="${PROJECTS[current]}"; n=$(title "$current"); v="${VIEWS[current]}"; printf '\033[H\033[2J'
  case "$action" in a) add_one "$p" "$n";; A) add_many "$p" "$n";; c|C) clear "$p" "$n" not-open;; r|R) select_prs "$p" "$n"&&remove_ids "$p" "${IDS[@]}";; x|X) clear "$p" "$n" all;; l|L) items "$p"|jq -r '.items[]|select(.content.type=="PullRequest")|"\(.content.repository) #\(.content.number)  \(.content.title)"';; v|V) [ -z "$v" ]||"${GHPROJ_BROWSER:-firefox}" "$v";; p|P) "${GHPROJ_BROWSER:-firefox}" "https://github.com/users/$PROJECT_OWNER/projects/$p";; m|M) move "$current" "$p" "$n";; esac
}
usage() { printf '%s\n' 'Usage: ghproj | ghproj list | ghproj add [pr-reference]' '       ghproj config <owner> <project> [view-url]' '       ghproj config add <project> [view-url] | remove <project>'; }

load_config
case "${1:-}" in
  help|-h|--help) usage;;
  config) shift; case "${1:-}" in
    add) PROJECTS+=("${2:?}"); VIEWS+=("${3:-}"); TITLES+=("");;
    remove) for i in "${!PROJECTS[@]}"; do [ "${PROJECTS[i]}" = "${2:?}" ]&&{ unset 'PROJECTS[i]' 'VIEWS[i]' 'TITLES[i]'; PROJECTS=("${PROJECTS[@]}"); VIEWS=("${VIEWS[@]}"); TITLES=("${TITLES[@]}"); }; done;;
    *) PROJECT_OWNER="${1:?}"; PROJECTS=("${2:?}"); VIEWS=("${3:-}"); TITLES=("");;
  esac; save_config;;
  "") command -v gum >/dev/null||exit 1; if [ "${#PROJECTS[@]}" = 0 ]; then PROJECT_OWNER=$(gum input --header owner)||exit; p=$(gum input --header project)||exit; PROJECTS=("$p"); VIEWS=(""); TITLES=(""); save_config; fi; for i in "${!PROJECTS[@]}"; do title "$i" >/dev/null; done; interactive;;
  list) items "${PROJECTS[0]}"|jq -r '.items[]|select(.content.type=="PullRequest")|"\(.content.repository) #\(.content.number)  \(.content.title)"';;
  add) shift; add_one "${PROJECTS[0]}" "$(title 0)" "$@";;
  *) usage >&2; exit 1;;
esac
