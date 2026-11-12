#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/toolshed/ghproj/config.yaml"
PROJECT_OWNER="${PROJECT_OWNER:-}"
declare -a PROJECTS=() VIEWS=() TITLES=() CUSTOM_FIELDS=() FIELD_DEFAULTS=() FIELD_SHORTCUTS=()

append_project() {
  PROJECTS+=("$1"); VIEWS+=("${2:-}"); TITLES+=("")
  CUSTOM_FIELDS+=(""); FIELD_DEFAULTS+=(""); FIELD_SHORTCUTS+=('{}')
}
yaml_scalar() {
  local value=$1
  case "$value" in
    \"*) jq -er 'select(type == "string")' <<< "$value";;
    \'*\') value=${value:1:${#value}-2}; printf '%s' "${value//\'\'/\'}";;
    *) printf '%s' "$value";;
  esac
}

load_config() {
  [ -f "$CONFIG_FILE" ] || return 0
  local record value key i=-1 p v
  : "${PROJECT_OWNER:=$(yaml_scalar "$(sed -n 's/^owner:[[:space:]]*//p' "$CONFIG_FILE" | head -1)")}"
  # Read the supported block-style YAML keys, tracking custom_field indentation.
  while IFS=$'\t' read -r record value; do
    case "$record" in
      project) append_project "$(yaml_scalar "$value")"; i=$((${#PROJECTS[@]}-1));;
      view_url) VIEWS[i]=$(yaml_scalar "$value");;
      name) CUSTOM_FIELDS[i]=$(yaml_scalar "$value");;
      default) FIELD_DEFAULTS[i]=$(yaml_scalar "$value");;
      shortcut)
        key=$(yaml_scalar "${value%%$'\t'*}")
        value=$(yaml_scalar "${value#*$'\t'}")
        FIELD_SHORTCUTS[i]=$(jq -c --arg key "$key" --arg value "$value" '. + {($key): $value}' <<< "${FIELD_SHORTCUTS[i]}")
        ;;
    esac
  done < <(awk '
    /^[[:space:]]*($|#)/ { next }
    {
      indent=match($0, /[^ ]/)-1
      line=$0; sub(/^[[:space:]]*/, "", line); sub(/[[:space:]\r]+$/, "", line)
      if (line ~ /^-[[:space:]]*project:/) {
        sub(/^-[[:space:]]*project:[[:space:]]*/, "", line)
        print "project\t" line; project=1; field=-1; shortcuts=-1; next
      }
      if (!project) next
      if (field >= 0 && indent <= field) { field=-1; shortcuts=-1 }
      if (shortcuts >= 0 && indent <= shortcuts) shortcuts=-1
      key=line; sub(/:.*/, "", key)
      value=line; sub(/^[^:]*:[[:space:]]*/, "", value)
      if (key == "custom_field") { field=indent; next }
      if (field >= 0) {
        if (key == "shortcuts") { shortcuts=indent; next }
        if (shortcuts >= 0) print "shortcut\t" key "\t" value
        else if (key == "name" || key == "default") print key "\t" value
      } else if (key == "view_url") print key "\t" value
    }' "$CONFIG_FILE")
  if [ "${#PROJECTS[@]}" -eq 0 ]; then
    p=$(sed -n 's/^project:[[:space:]]*//p' "$CONFIG_FILE"|head -1); v=$(sed -n 's/^view_url:[[:space:]]*//p' "$CONFIG_FILE"|head -1)
    [ -z "$p" ] || append_project "$p" "$v"
    p=$(sed -n 's/^secondary_project:[[:space:]]*//p' "$CONFIG_FILE"|head -1)
    [ -z "$p" ] || append_project "$p"
  fi
}
save_config() {
  local i; mkdir -p "$(dirname "$CONFIG_FILE")"
  { printf 'owner: %s\nprojects:\n' "$PROJECT_OWNER"; for i in "${!PROJECTS[@]}"; do
      printf '  - project: %s\n' "${PROJECTS[i]}"; [ -z "${VIEWS[i]}" ] || printf '    view_url: %s\n' "${VIEWS[i]}"
      if [ -n "${CUSTOM_FIELDS[i]}" ]; then
        printf '    custom_field:\n      name: %s\n' "$(jq -Rn --arg v "${CUSTOM_FIELDS[i]}" '$v')"
        [ -z "${FIELD_DEFAULTS[i]}" ] || printf '      default: %s\n' "$(jq -Rn --arg v "${FIELD_DEFAULTS[i]}" '$v')"
        if [ "${FIELD_SHORTCUTS[i]}" != '{}' ]; then
          printf '      shortcuts:\n'
          jq -r 'to_entries[] | "        \(.key | tojson): \(.value | tojson)"' <<< "${FIELD_SHORTCUTS[i]}"
        fi
      fi
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
field_error() { printf 'Custom field: %s\n' "$*" >&2; return 1; }
with_progress() {
  local message=$1; shift
  printf '… %s\n' "$message" >&2
  if [ -t 2 ] && command -v gum >/dev/null 2>&1; then
    gum spin --title "$message" --show-stdout --show-stderr -- "$@"
  else
    "$@"
  fi
}
choose_field_value() {
  local name=$1 default=$2 shortcuts=$3 selected=0 i key rest shortcut
  shift 3
  local -a options=("$@")
  [ -t 0 ] || field_error 'selection requires an interactive terminal.' || return
  for i in "${!options[@]}"; do [ "${options[i]}" != "$default" ] || selected=$i; done
  while :; do
    printf '\033[H\033[2J%s\n\n' "$name" >&2
    for i in "${!options[@]}"; do
      shortcut=$(jq -r --arg value "${options[i]}" 'to_entries | map(select(.value == $value) | .key) | join("/")' <<< "$shortcuts")
      if [ "$i" = "$selected" ]; then printf '\033[38;5;212m❯ ' >&2; else printf '  ' >&2; fi
      [ -z "$shortcut" ] || printf '[%s] ' "$shortcut" >&2
      printf '%s\033[0m\n' "${options[i]}" >&2
    done
    printf '\n↑/↓ or j/k navigate · enter select · digit shortcut selects · esc/ctrl-c cancel\n' >&2
    IFS= read -rsn1 key || return 130
    case "$key" in
      j) selected=$(((selected+1)%${#options[@]}));;
      k) selected=$(((selected-1+${#options[@]})%${#options[@]}));;
      $'\e')
        rest=''; IFS= read -rsn2 -t 0.1 rest || true
        case "$rest" in
          '[A') selected=$(((selected-1+${#options[@]})%${#options[@]}));;
          '[B') selected=$(((selected+1)%${#options[@]}));;
          *) return 130;;
        esac;;
      $'\003') return 130;;
      ''|$'\n'|$'\r') printf '%s' "${options[selected]}"; return;;
      [0-9])
        if shortcut=$(jq -er --arg key "$key" '.[$key] // empty' <<< "$shortcuts"); then
          printf '%s' "$shortcut"; return
        fi;;
    esac
  done
}
prepare_field() {
  local p=$1 context=$2 i index=-1 field_id project_id option_id metadata kind name default shortcuts value
  local -a options=()
  FIELD_EDIT_ARGS=()
  FIELD_LABEL=''; FIELD_VALUE=''
  for i in "${!PROJECTS[@]}"; do
    if [ "${PROJECTS[i]}" = "$p" ]; then index=$i; break; fi
  done
  [ "$index" -ge 0 ] && [ -n "${CUSTOM_FIELDS[index]}" ] || return 0
  name=${CUSTOM_FIELDS[index]}; default=${FIELD_DEFAULTS[index]}; shortcuts=${FIELD_SHORTCUTS[index]}
  field_id=$(with_progress "Loading $name for $context…" gh project field-list "$p" --owner "$PROJECT_OWNER" --format json --limit 1000 |
    jq -er --arg name "$name" '[.fields[] | select(.name == $name)] | if length == 1 then .[0].id else empty end') || {
      field_error "cannot resolve '$name' in project #$p."; return 1;
    }
  metadata=$(with_progress "Loading $name options…" gh api graphql -f query='query($id: ID!) {
    node(id: $id) {
      ... on ProjectV2FieldCommon { id name dataType project { id } }
      ... on ProjectV2SingleSelectField { options { id name } }
    }
  }' -f id="$field_id" --jq '.data.node') || return
  kind=$(jq -er '.dataType' <<< "$metadata") || return
  project_id=$(jq -er '.project.id' <<< "$metadata") || return
  FIELD_EDIT_ARGS=(--project-id "$project_id" --field-id "$field_id")
  case "$kind" in
    SINGLE_SELECT)
      mapfile -t options < <(jq -r '.options[].name' <<< "$metadata")
      [ "${#options[@]}" -gt 0 ] || { field_error "'$name' has no options."; return 1; }
      if [ -n "$default" ] && ! jq -e --arg value "$default" 'any(.options[]; .name == $value)' <<< "$metadata" >/dev/null; then
        field_error "default '$default' is not an option for '$name'."; return 1
      fi
      if ! jq -e --argjson field "$metadata" 'to_entries | all(.[]; (.key | test("^[0-9]$")) and (.value as $v | any($field.options[]; .name == $v)))' <<< "$shortcuts" >/dev/null; then
        field_error "shortcuts for '$name' must map single digits to existing option names."; return 1
      fi
      value=$(choose_field_value "$name — $context" "$default" "$shortcuts" "${options[@]}") || return
      option_id=$(jq -er --arg value "$value" '.options[] | select(.name == $value) | .id' <<< "$metadata") || return
      FIELD_EDIT_ARGS+=(--single-select-option-id "$option_id")
      ;;
    TEXT|NUMBER|DATE)
      [ "$shortcuts" = '{}' ] || { field_error "shortcuts require a single-select field."; return 1; }
      while :; do
        value=$(gum input --header "$name ($kind) — $context" --value "$default") || return
        case "$kind" in
          TEXT) FIELD_EDIT_ARGS+=(--text "$value"); break;;
          NUMBER)
            if [[ $value =~ ^-?([0-9]+([.][0-9]*)?|[.][0-9]+)([eE][+-]?[0-9]+)?$ ]]; then
              FIELD_EDIT_ARGS+=(--number "$value"); break
            fi
            printf 'Enter a valid number.\n' >&2;;
          DATE)
            if [[ $value =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] && [ "$(date -d "$value" +%F 2>/dev/null)" = "$value" ]; then
              FIELD_EDIT_ARGS+=(--date "$value"); break
            fi
            printf 'Enter a valid date (YYYY-MM-DD).\n' >&2;;
        esac
        default=$value
      done;;
    *) field_error "unsupported type '$kind' for '$name'."; return 1;;
  esac
  FIELD_LABEL=$name; FIELD_VALUE=$value
  if [ -t 2 ]; then printf '\033[H\033[2J' >&2; fi
  printf '%s: %s\n\n' "$FIELD_LABEL" "$FIELD_VALUE" >&2
}
add_prepared() {
  local p=$1 url=$2 item_id
  if [ "${#FIELD_EDIT_ARGS[@]}" = 0 ]; then
    if ! with_progress "Adding PR to project #$p: $url" gh project item-add "$p" --owner "$PROJECT_OWNER" --url "$url"; then
      printf 'Failed to add PR to project #%s: %s\n' "$p" "$url" >&2
      return 1
    fi
    printf '✓ PR added to project #%s: %s\n' "$p" "$url" >&2
    return
  fi
  item_id=$(with_progress "Adding PR to project #$p: $url" gh project item-add "$p" --owner "$PROJECT_OWNER" --url "$url" --format json --jq '.id') || {
    printf 'Failed to add PR to project #%s: %s\n' "$p" "$url" >&2
    return 1
  }
  if ! with_progress "Setting $FIELD_LABEL to $FIELD_VALUE…" gh project item-edit --id "$item_id" "${FIELD_EDIT_ARGS[@]}"; then
    printf 'PR is in project #%s, but setting its custom field failed: %s\n' "$p" "$url" >&2
    return 1
  fi
  printf '✓ PR added to project #%s with %s: %s\n  %s\n' "$p" "$FIELD_LABEL" "$FIELD_VALUE" "$url" >&2
}
add_one() {
  local p=$1 n=$2 ref clipboard=""; local -a refs=(); shift 2
  if [ $# = 0 ]; then
    if command -v wl-paste >/dev/null 2>&1; then
      clipboard=$(wl-paste --no-newline 2>/dev/null || true)
      clipboard=${clipboard%$'\r'}
      parse_pr "$clipboard" 2>/dev/null || clipboard=""
    fi
    ref=$(gum input --value "$clipboard" --header "PR to add to \"$n\"" --prompt '❯ ' --placeholder 'owner/repo#123') || return
    read -ra refs <<< "$ref"
    parse_pr "${refs[@]}" || return 1
  else parse_pr "$@" || return 1; fi
  prepare_field "$p" "$n" || return
  add_prepared "$p" "https://github.com/$REPO/pull/$NUM"
}
add_many() {
  local p=$1 n=$2 ref text; shift 2; text=$(gum write --header "PRs to add to \"$n\"" --placeholder 'Paste PR links, separated by commas, spaces, or newlines') || return
  [ -n "$text" ] || return 0
  prepare_field "$p" "all PRs in $n" || return
  while IFS= read -r ref; do
    parse_pr "$ref" || { printf 'Invalid PR reference: %s\n' "$ref" >&2; return 1; }
    add_prepared "$p" "https://github.com/$REPO/pull/$NUM" || return
  done < <(printf '%s\n' "$text"|tr ',[:space:]' '\n'|sed '/^$/d')
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
    add) append_project "${2:?}" "${3:-}";;
    remove)
      for i in "${!PROJECTS[@]}"; do
        if [ "${PROJECTS[i]}" = "${2:?}" ]; then
          unset 'PROJECTS[i]' 'VIEWS[i]' 'TITLES[i]' 'CUSTOM_FIELDS[i]' 'FIELD_DEFAULTS[i]' 'FIELD_SHORTCUTS[i]'
        fi
      done
      PROJECTS=("${PROJECTS[@]}"); VIEWS=("${VIEWS[@]}"); TITLES=("${TITLES[@]}")
      CUSTOM_FIELDS=("${CUSTOM_FIELDS[@]}"); FIELD_DEFAULTS=("${FIELD_DEFAULTS[@]}"); FIELD_SHORTCUTS=("${FIELD_SHORTCUTS[@]}")
      ;;
    *) PROJECT_OWNER="${1:?}"; PROJECTS=(); VIEWS=(); TITLES=(); CUSTOM_FIELDS=(); FIELD_DEFAULTS=(); FIELD_SHORTCUTS=(); append_project "${2:?}" "${3:-}";;
  esac; save_config;;
  "") command -v gum >/dev/null||exit 1; if [ "${#PROJECTS[@]}" = 0 ]; then PROJECT_OWNER=$(gum input --header owner)||exit; p=$(gum input --header project)||exit; append_project "$p"; save_config; fi; for i in "${!PROJECTS[@]}"; do title "$i" >/dev/null; done; interactive;;
  list) items "${PROJECTS[0]}"|jq -r '.items[]|select(.content.type=="PullRequest")|"\(.content.repository) #\(.content.number)  \(.content.title)"';;
  add) shift; add_one "${PROJECTS[0]}" "$(title 0)" "$@";;
  *) usage >&2; exit 1;;
esac
