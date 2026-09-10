#!/usr/bin/env bash
# ghproj — manage PRs in a GitHub Project (v2) via `gh`. Run `ghproj help` for usage.
set -euo pipefail

CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/toolshed/ghproj/config.yaml"

load_config() {
  [ -f "$CONFIG_FILE" ] || return 0
  local o n v
  o=$(sed -n 's/^owner:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  n=$(sed -n 's/^number:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  v=$(sed -n 's/^view_id:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  : "${PROJECT_OWNER:=$o}"
  : "${PROJECT_NUMBER:=$n}"
  : "${PROJECT_VIEW_ID:=$v}"
}

save_config() {
  mkdir -p "$(dirname "$CONFIG_FILE")"
  printf 'owner: %s\nnumber: %s\nview_id: %s\n' "$1" "$2" "${3:-}" > "$CONFIG_FILE"
}

# parse_pr accepts a PR reference in any of these forms and sets REPO/NUM,
# returning non-zero if it can't be parsed (does not exit):
#   <url>                 https://github.com/owner/repo/pull/372
#   <owner/repo#pr>        openshift/oauth-proxy#372
#   <owner/repo> <pr>      openshift/oauth-proxy 372
#   <owner> <repo> <pr>    openshift oauth-proxy 372
parse_pr() {
  case $# in
    1)
      if [[ $1 =~ ^(https?://)?github\.com/([^/]+/[^/]+)/pull/([0-9]+)/?$ ]]; then
        REPO="${BASH_REMATCH[2]}"; NUM="${BASH_REMATCH[3]}"
      elif [[ $1 =~ ^([^/]+/[^/]+)#([0-9]+)$ ]]; then
        REPO="${BASH_REMATCH[1]}"; NUM="${BASH_REMATCH[2]}"
      else
        return 1
      fi
      ;;
    2) REPO=$1; NUM=$2 ;;
    3) REPO="$1/$2"; NUM=$3 ;;
    *) return 1 ;;
  esac
}

usage() {
  cat <<'EOF'
ghproj — manage PRs in a GitHub Project (v2) via `gh`

Usage:
  ghproj        interactive mode
  ghproj list
  ghproj add    [pr-reference]
  ghproj remove <pr-reference>
  ghproj clear  [closed|merged|not-open|all]
  ghproj config <owner> <number> [view-id]
  ghproj help

pr-reference (any of):
  https://github.com/owner/repo/pull/372
  owner/repo#372
  owner/repo 372
  owner repo 372
  omit it on `add` to be prompted interactively

Commands:
  list           show every PR item currently in the project
  add            add a PR to the project (prompts if none given)
  remove         remove one PR from the project
  clear [mode]   bulk-remove PR items by state (default: not-open):
                   closed     state == CLOSED (closed without merging)
                   merged     state == MERGED
                   not-open   state != OPEN (closed or merged)
                   all        every PR item, regardless of state
  config         save owner/number/view-id to the config file (see below)
  help           show this message

Env (override the config file):
  PROJECT_OWNER   user or org login that owns the project (e.g. "octocat")
  PROJECT_NUMBER  project number, from its URL (…/projects/<N>)
  GHPROJ_BROWSER   browser executable for v/p shortcuts (default: firefox)

Config file (used when the env vars above aren't set):
  ~/.config/toolshed/ghproj/config.yaml
    owner: octocat
    number: 5
    view_id: 2688867
  Write it with: ghproj config <owner> <number> [view-id]

Requires:
  gh: authenticated, with `project` scope
  jq: for parsing gh responses
  gum: for the interactive prompts and menu
  wl-paste: for checking clipboard for an already pasted PR string
EOF
}

case "${1:-}" in
  help|-h|--help)
    usage
    exit 0
    ;;
  config)
    owner=${2:?usage: ghproj config <owner> <number> [view-id]}
    number=${3:?usage: ghproj config <owner> <number> [view-id]}
    view_id=${4:-}
    save_config "$owner" "$number" "$view_id"
    echo "saved to $CONFIG_FILE"
    exit 0
    ;;
esac

load_config
if [ -z "${1:-}" ]; then
  command -v gum >/dev/null 2>&1 || {
    echo "interactive mode requires gum (https://github.com/charmbracelet/gum)" >&2
    exit 1
  }

  config_exists=0
  [ -f "$CONFIG_FILE" ] && config_exists=1

  if [ "$config_exists" -eq 0 ] || [ -z "${PROJECT_OWNER:-}" ] || [ -z "${PROJECT_NUMBER:-}" ]; then
    PROJECT_OWNER=$(gum input --value "${PROJECT_OWNER:-}" \
      --header "GitHub project owner" --header.foreground 255 \
      --prompt "❯ " --placeholder "" --cursor.mode blink) || exit 0
    PROJECT_NUMBER=$(gum input --value "${PROJECT_NUMBER:-}" \
      --header "GitHub project number" --header.foreground 255 \
      --prompt "❯ " --placeholder "" --cursor.mode blink) || exit 0
    PROJECT_VIEW_ID=$(gum input --value "${PROJECT_VIEW_ID:-}" \
      --header "Pull-request view ID (optional)" --header.foreground 255 \
      --prompt "❯ " --placeholder "" --cursor.mode blink) || exit 0
    save_config "$PROJECT_OWNER" "$PROJECT_NUMBER" "${PROJECT_VIEW_ID:-}"
  fi
fi
: "${PROJECT_OWNER:?set PROJECT_OWNER or run: ghproj config <owner> <number>}"
: "${PROJECT_NUMBER:?set PROJECT_NUMBER or run: ghproj config <owner> <number>}"

gh_query() {
  local title=$1
  shift
  if [ -t 2 ] && command -v gum >/dev/null 2>&1; then
    gum spin --spinner line --title "$title" --show-stdout --show-error -- "$@"
  else
    "$@"
  fi
}

items() {
  gh_query "Loading PRs…" gh project item-list "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json --limit 200
}

gh_action() {
  local title=$1
  shift
  if [ -t 2 ] && command -v gum >/dev/null 2>&1; then
    gum spin --spinner line --title "$title" --show-error -- "$@"
  else
    "$@"
  fi
}

remove_items() {
  local id
  for id in "$@"; do
    gh_action "Removing PR…" gh project item-delete "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --id "$id"
  done
}

project_name() {
  if [ -z "${PROJECT_NAME:-}" ]; then
    if ! PROJECT_NAME=$(gh_query "Loading project…" gh project view "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json -q .title); then
      echo "error: could not resolve project (owner=\"$PROJECT_OWNER\" number=\"$PROJECT_NUMBER\"): $PROJECT_NAME" >&2
      return 1
    fi
  fi
  echo "$PROJECT_NAME"
}

pr_state() {
  gh_query "Loading PR status…" gh pr view "$1" --json state,mergedAt \
    -q 'if .mergedAt == null then .state else "MERGED" end'
}

status_label() {
  case "$1" in
    OPEN)    printf '\033[32mOPEN\033[0m' ;;
    CLOSED)  printf '\033[33mCLOSED\033[0m' ;;
    MERGED)  printf '\033[35mMERGED\033[0m' ;;
    *)       printf '\033[90mUNKNOWN\033[0m' ;;
  esac
}

list_command() {
  items | jq -r '.items[] | select(.content.type=="PullRequest") |
    "\(.content.repository) #\(.content.number)  \(.content.title)"'
}

add_command() {
    local default clip ref
    local -a args clip_args

    if [ $# -eq 0 ]; then
      default=""
      if command -v wl-paste >/dev/null 2>&1; then
        clip=$(wl-paste 2>/dev/null || true)
        if [ -n "$clip" ]; then
          read -ra clip_args <<< "$clip"
          parse_pr "${clip_args[@]}" 2>/dev/null && default="$clip"
        fi
      fi
      if ! project_name >/dev/null; then
        PROJECT_NAME="$PROJECT_OWNER/$PROJECT_NUMBER"
      fi
      ref=$(gum input --value "$default" --prompt "❯ " \
        --header "PR to add to \"$PROJECT_NAME\"" \
        --header.foreground 255 --prompt.foreground 212 \
        --placeholder "owner/repo#123") || return 1
      read -ra args <<< "$ref"
      parse_pr "${args[@]}" || { echo "cannot parse PR reference: $ref" >&2; exit 1; }
    else
      parse_pr "$@" || { echo "cannot parse PR reference: $*" >&2; exit 1; }
    fi
    gh_action "Adding PR…" gh project item-add "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" \
      --url "https://github.com/$REPO/pull/$NUM"
}

remove_command() {
    parse_pr "$@" || { echo "cannot parse PR reference: $*" >&2; exit 1; }
    id=$(items | jq -r --arg repo "$REPO" --arg num "$NUM" \
      '.items[] | select(.content.repository==$repo and (.content.number|tostring)==$num) | .id')
    [ -z "$id" ] && { echo "not found: $REPO #$NUM"; exit 1; }
    remove_items "$id"
}

choose_remove_command() {
    local selection choice index id repo number title url state
    local -a rows choices selected_ids
    declare -A seen=()

    mapfile -t rows < <(items | jq -r '.items[] | select(.content.type=="PullRequest") |
      [.id, .content.repository, .content.number, .content.title, .content.url] | @tsv')

    if [ "${#rows[@]}" -eq 0 ]; then
      echo "no pull requests in the project"
      return 0
    fi

    local list_text=""
    for index in "${!rows[@]}"; do
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      state=$(pr_state "$url" || echo UNKNOWN)
      colored_state=$(status_label "$state")
      list_text+=$(printf '[%d] %-7s %s #%s  %s' "$((index + 1))" "$colored_state" "$repo" "$number" "$title")
      list_text+=$'\n'
    done

    gum style --no-strip-ansi --bold "Pull requests" "$list_text"
    selection=$(gum input --prompt "❯ " \
      --header "PRs to remove (comma-separated)" \
      --placeholder "1,3,5") || return 1
    IFS=',' read -ra choices <<< "$selection"
    for choice in "${choices[@]}"; do
      choice=${choice//[[:space:]]/}
      if ! [[ $choice =~ ^[0-9]+$ ]]; then
        echo "invalid selection: $choice" >&2
        return 1
      fi
      index=$((choice - 1))
      if [ "$index" -lt 0 ] || [ "$index" -ge "${#rows[@]}" ]; then
        echo "selection out of range: $choice" >&2
        return 1
      fi
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      if [ -z "${seen[$id]+x}" ]; then
        selected_ids+=("$id")
        seen[$id]=1
      fi
    done

    remove_items "${selected_ids[@]}"
}

list_interactive_command() {
    local row repo number title url state colored_state list_text
    local -a rows

    mapfile -t rows < <(items | jq -r '.items[] | select(.content.type=="PullRequest") |
      [.content.repository, .content.number, .content.title, .content.url] | @tsv')

    list_text="PRs in \"$PROJECT_NAME\""
    list_text+=$'\n\n'
    for row in "${rows[@]}"; do
      IFS=$'\t' read -r repo number title url <<< "$row"
      state=$(pr_state "$url" || echo UNKNOWN)
      colored_state=$(status_label "$state")
      list_text+=$(printf '%-7s %s #%s  %s' "$colored_state" "$repo" "$number" "$title")
      list_text+=$'\n'
    done

    printf '\033[H\033[2J%s\n' "$list_text"
    printf '\033[38;5;241mpress any key to exit\033[0m\n'
    IFS= read -rsn1 key || true
    printf '\033[H\033[2J'
}

clear_command() {
    local mode=${1:-not-open}
    while read -r id url; do
        if [ "$mode" = "all" ]; then
          match=1
        else
          state=$(pr_state "$url")
          case "$mode" in
            closed)   [ "$state" = "CLOSED" ] && match=1 || match=0 ;;
            merged)   [ "$state" = "MERGED" ] && match=1 || match=0 ;;
            not-open) [ "$state" != "OPEN" ] && match=1 || match=0 ;;
            *) echo "unknown mode: $mode (want closed|merged|not-open|all)" >&2; exit 1 ;;
          esac
        fi
        if [ "$match" = 1 ]; then
          gh_action "Clearing PRs…" gh project item-delete "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --id "$id"
        fi
      done < <(items | jq -r '.items[] | select(.content.type=="PullRequest") |
        "\(.id) \(.content.url)"')
}

open_url() {
    "${GHPROJ_BROWSER:-firefox}" "$1"
}

open_pulls_url() {
    if [ -z "${PROJECT_VIEW_ID:-}" ]; then
      echo "set view_id in $CONFIG_FILE to open the pull-request view" >&2
      return 1
    fi
    open_url "https://github.com/pulls/$PROJECT_VIEW_ID"
}

open_project_url() {
  open_url "https://github.com/users/$PROJECT_OWNER/projects/$PROJECT_NUMBER"
}

success_exit() {
  printf 'success\n'
  sleep 1
  exit 0
}

interactive_menu() {
    local index=0 key rest i menu_text
    local -a options=(
      "a  Add a PR"
      "c  Clear all closed and merged PRs"
      "r  Choose PR(s) to remove"
      "x  Clear all PRs"
      "l  List all PRs"
      "v  Open PR view"
      "p  Open project view"
      "q  Quit without any changes"
    )

    while true; do
      menu_text="Manage PRs for \"$PROJECT_NAME\""
      menu_text+=$'\n\n'
      for i in "${!options[@]}"; do
        if [ "$i" -eq "$index" ]; then
          menu_text+=$'\033[38;5;212m❯ '
          menu_text+="${options[i]}"
          menu_text+=$'\033[0m\n'
        else
          menu_text+="  ${options[i]}"$'\n'
        fi
        case "$i" in
          0|3|6) menu_text+=$'\n' ;;
        esac
      done
      menu_text+=$'\n'
      menu_text+=$'\033[38;5;241m↑/↓ or j/k navigate · enter select · 1/2/3/a/l/v/p/q shortcuts\033[0m\n'
      printf '\033[H'
      printf '%s\n' "$menu_text"

      IFS= read -rsn1 key || { MENU_CHOICE=q; return; }
      case "$key" in
        $'\e')
          IFS= read -rsn2 rest || { MENU_CHOICE=q; return; }
          case "$rest" in
            '[A'|'[D') if (( index > 0 )); then index=$((index - 1)); fi ;;
            '[B'|'[C') if (( index < ${#options[@]} - 1 )); then index=$((index + 1)); fi ;;
          esac
          ;;
        k) if (( index > 0 )); then index=$((index - 1)); fi ;;
        j) if (( index < ${#options[@]} - 1 )); then index=$((index + 1)); fi ;;
        1|2|3|a|A|l|L|v|V|p|P|q|Q) MENU_CHOICE=$key; return ;;
        $'\n'|$'\r') MENU_CHOICE=${options[index]:0:1}; return ;;
      esac
    done
}

case "${1:-}" in
  "")
    command -v gum >/dev/null 2>&1 || {
      echo "interactive mode requires gum (https://github.com/charmbracelet/gum)" >&2
      exit 1
    }
    if ! project_name >/dev/null; then
      PROJECT_NAME="$PROJECT_OWNER/$PROJECT_NUMBER"
    fi
    interactive_menu
    printf '\033[H\033[2J'
    case "$MENU_CHOICE" in
      a|A)
        if add_command; then success_exit; fi
        ;;
      c|C)
        if gum confirm "Clear all closed and merged PRs?"; then
          if clear_command not-open; then success_exit; fi
        fi
        ;;
      r|R)
        if choose_remove_command; then success_exit; fi
        ;;
      x|X)
        if gum confirm "Clear all PRs?"; then
          if clear_command all; then success_exit; fi
        fi
        ;;
      l|L) list_interactive_command ;;
      v|V) open_pulls_url ;;
      p|P) open_project_url ;;
      q|Q) exit 0 ;;
      *) echo "unknown choice: $MENU_CHOICE" >&2; exit 1 ;;
    esac
    ;;

  list)
    list_command
    ;;

  add)
    shift
    add_command "$@"
    ;;

  remove)
    shift
    remove_command "$@"
    ;;

  clear)
    shift
    clear_command "${1:-not-open}"
    ;;

  *)
    usage >&2
    exit 1
    ;;
esac
