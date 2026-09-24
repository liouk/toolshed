#!/usr/bin/env bash
# ghproj — manage PRs in a GitHub Project (v2) via `gh`. Run `ghproj help` for usage.
set -euo pipefail

CONFIG_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/toolshed/ghproj/config.yaml"

load_config() {
  [ -f "$CONFIG_FILE" ] || return 0
  local o n v m
  o=$(sed -n 's/^owner:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  n=$(sed -n 's/^project:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  v=$(sed -n 's/^view_id:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  m=$(sed -n 's/^secondary_project:[[:space:]]*//p' "$CONFIG_FILE" | head -n1)
  : "${PROJECT_OWNER:=$o}"
  : "${PROJECT_NUMBER:=$n}"
  : "${PROJECT_VIEW_ID:=$v}"
  : "${SECONDARY_PROJECT_NUMBER:=$m}"
}

save_config() {
  mkdir -p "$(dirname "$CONFIG_FILE")"
  printf 'owner: %s\nproject: %s\nview_id: %s\nsecondary_project: %s\n' "$1" "$2" "${3:-}" "${4:-}" > "$CONFIG_FILE"
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
  ghproj config <owner> <number> [view-id] [secondary-project-number]
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
  SECONDARY_PROJECT_NUMBER  default secondary project for adding or moving PRs
  GHPROJ_BROWSER   browser executable for v/p shortcuts (default: firefox)

Config file (used when the env vars above aren't set):
  ~/.config/toolshed/ghproj/config.yaml
    owner: octocat
    project: 5
    view_id: 2688867
    secondary_project: 6
  Write it with: ghproj config <owner> <number> [view-id] [secondary-project-number]

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
    owner=${2:?usage: ghproj config <owner> <number> [view-id] [secondary-project-number]}
    number=${3:?usage: ghproj config <owner> <number> [view-id] [secondary-project-number]}
    view_id=${4:-}
    secondary_project=${5:-}
    save_config "$owner" "$number" "$view_id" "$secondary_project"
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
    save_config "$PROJECT_OWNER" "$PROJECT_NUMBER" "${PROJECT_VIEW_ID:-}" "${SECONDARY_PROJECT_NUMBER:-}"
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

secondary_project_name() {
  [ -n "${SECONDARY_PROJECT_NUMBER:-}" ] || return 0
  if ! SECONDARY_PROJECT_NAME=$(gh_query "Loading secondary project…" gh project view "$SECONDARY_PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json -q .title); then
    SECONDARY_PROJECT_NAME="project #$SECONDARY_PROJECT_NUMBER"
  fi
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

add_to_project_command() {
    local target_number=$1 target_name=$2 default clip ref
    local -a args clip_args
    shift 2

    if [ $# -eq 0 ]; then
      default=""
      if command -v wl-paste >/dev/null 2>&1; then
        clip=$(wl-paste 2>/dev/null || true)
        if [ -n "$clip" ]; then
          read -ra clip_args <<< "$clip"
          parse_pr "${clip_args[@]}" 2>/dev/null && default="$clip"
        fi
      fi
      ref=$(gum input --value "$default" --prompt "❯ " \
        --header "PR to add to \"$target_name\"" \
        --header.foreground 255 --prompt.foreground 212 \
        --placeholder "owner/repo#123") || return 1
      read -ra args <<< "$ref"
      parse_pr "${args[@]}" || { echo "cannot parse PR reference: $ref" >&2; exit 1; }
    else
      parse_pr "$@" || { echo "cannot parse PR reference: $*" >&2; exit 1; }
    fi
    gh_action "Adding PR…" gh project item-add "$target_number" --owner "$PROJECT_OWNER" \
      --url "https://github.com/$REPO/pull/$NUM"
}

add_command() {
    if ! project_name >/dev/null; then
      PROJECT_NAME="$PROJECT_OWNER/$PROJECT_NUMBER"
    fi
    add_to_project_command "$PROJECT_NUMBER" "$PROJECT_NAME" "$@"
}

add_multiple_to_project_command() {
    local target_number=$1 target_name=$2 refs_text ref failures=0 added=0
    local -a refs
    shift 2

    refs_text=$(gum write \
      --header "PRs to add to \"$target_name\"" \
      --header.foreground 255 \
      --placeholder "Paste PR links, separated by commas, spaces, or newlines") || return 1

    mapfile -t refs < <(printf '%s' "$refs_text" | tr ',[:space:]' '\n' | sed '/^$/d')
    if [ "${#refs[@]}" -eq 0 ]; then
      echo "No PRs provided."
      return 1
    fi

    for ref in "${refs[@]}"; do
      if ! parse_pr "$ref"; then
        echo "cannot parse PR reference: $ref" >&2
        failures=$((failures + 1))
        continue
      fi
      if gh_action "Adding PR $REPO #$NUM…" gh project item-add "$target_number" --owner "$PROJECT_OWNER" \
        --url "https://github.com/$REPO/pull/$NUM"; then
        added=$((added + 1))
      else
        failures=$((failures + 1))
      fi
    done

    echo "Added $added PR(s)."
    [ "$failures" -eq 0 ]
}

add_multiple_command() {
    if ! project_name >/dev/null; then
      PROJECT_NAME="$PROJECT_OWNER/$PROJECT_NUMBER"
    fi
    add_multiple_to_project_command "$PROJECT_NUMBER" "$PROJECT_NAME"
}

select_secondary_project() {
    SELECTED_SECONDARY_PROJECT_NUMBER=${SECONDARY_PROJECT_NUMBER:-}
    SELECTED_SECONDARY_PROJECT_NAME=${SECONDARY_PROJECT_NAME:-}
    if [ -z "$SELECTED_SECONDARY_PROJECT_NUMBER" ]; then
      SELECTED_SECONDARY_PROJECT_NUMBER=$(gum input --header "Secondary project number" --prompt "❯ " --placeholder "Project number") || return 1
    fi
    if ! [[ $SELECTED_SECONDARY_PROJECT_NUMBER =~ ^[0-9]+$ ]]; then
      echo "invalid secondary project number: $SELECTED_SECONDARY_PROJECT_NUMBER" >&2
      return 1
    fi
    if [ -z "$SELECTED_SECONDARY_PROJECT_NAME" ]; then
      if ! SELECTED_SECONDARY_PROJECT_NAME=$(gh_query "Loading secondary project…" gh project view "$SELECTED_SECONDARY_PROJECT_NUMBER" --owner "$PROJECT_OWNER" --format json -q .title); then
        SELECTED_SECONDARY_PROJECT_NAME="project #$SELECTED_SECONDARY_PROJECT_NUMBER"
      fi
    fi
}

add_to_tracker_command() {
    select_secondary_project || return 1
    add_to_project_command "$SELECTED_SECONDARY_PROJECT_NUMBER" "$SELECTED_SECONDARY_PROJECT_NAME"
}

add_multiple_to_tracker_command() {
    select_secondary_project || return 1
    add_multiple_to_project_command "$SELECTED_SECONDARY_PROJECT_NUMBER" "$SELECTED_SECONDARY_PROJECT_NAME"
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

    for index in "${!rows[@]}"; do
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      state=$(pr_state "$url" || echo UNKNOWN)
      choices+=("$(printf '[%d] %-7s %s #%s  %s' \
        "$((index + 1))" "$state" "$repo" "$number" "$title")")
    done

    if ! selection=$(printf '%s\n' "${choices[@]}" | gum filter \
      --no-limit \
      --header "PRs to remove" \
      --placeholder "Filter PRs..." \
      --height 15 \
      --reverse); then
      return 1
    fi

    while IFS= read -r choice; do
      [ -z "$choice" ] && continue
      if [[ $choice =~ ^\[([0-9]+)\] ]]; then
        index=$((BASH_REMATCH[1] - 1))
      else
        echo "invalid selection: $choice" >&2
        return 1
      fi

      if [ "$index" -lt 0 ] || [ "$index" -ge "${#rows[@]}" ]; then
        echo "selection out of range: $((index + 1))" >&2
        return 1
      fi
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      if [ -z "${seen[$id]+x}" ]; then
        selected_ids+=("$id")
        seen[$id]=1
      fi
    done <<< "$selection"

    [ "${#selected_ids[@]}" -gt 0 ] && remove_items "${selected_ids[@]}"
}

choose_remove_secondary_command() {
    local original_number=$PROJECT_NUMBER original_name=${PROJECT_NAME:-} status
    select_secondary_project || return 1
    PROJECT_NUMBER=$SELECTED_SECONDARY_PROJECT_NUMBER
    PROJECT_NAME=$SELECTED_SECONDARY_PROJECT_NAME
    choose_remove_command
    status=$?
    PROJECT_NUMBER=$original_number
    PROJECT_NAME=$original_name
    return "$status"
}

move_items() {
    local target_number=$1 id url moved=0 failures=0
    shift

    while IFS=$'\t' read -r id url; do
      if gh_action "Adding PR to project #$target_number…" gh project item-add "$target_number" --owner "$PROJECT_OWNER" --url "$url"; then
        if gh_action "Removing PR from this project…" gh project item-delete "$PROJECT_NUMBER" --owner "$PROJECT_OWNER" --id "$id"; then
          moved=$((moved + 1))
        else
          failures=$((failures + 1))
        fi
      else
        failures=$((failures + 1))
      fi
    done <<< "$(printf '%s\n' "$@")"

    echo "Moved $moved PR(s) to project #$target_number."
    [ "$failures" -eq 0 ]
}

choose_move_command() {
    local target_number target_name selection choice index id repo number title url state
    local -a rows choices selected
    declare -A seen=()

    select_secondary_project || return 1
    target_number=$SELECTED_SECONDARY_PROJECT_NUMBER
    target_name=$SELECTED_SECONDARY_PROJECT_NAME
    if [ "$target_number" = "$PROJECT_NUMBER" ]; then
      echo "secondary project must be different from the current project" >&2
      return 1
    fi

    mapfile -t rows < <(items | jq -r '.items[] | select(.content.type=="PullRequest") |
      [.id, .content.repository, .content.number, .content.title, .content.url] | @tsv')
    if [ "${#rows[@]}" -eq 0 ]; then
      echo "no pull requests in the project"
      return 0
    fi

    for index in "${!rows[@]}"; do
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      state=$(pr_state "$url" || echo UNKNOWN)
      choices+=("$(printf '[%d] %-7s %s #%s  %s' \
        "$((index + 1))" "$state" "$repo" "$number" "$title")")
    done
    if ! selection=$(printf '%s\n' "${choices[@]}" | gum filter \
      --no-limit \
      --header "PRs to move to \"$target_name\"" \
      --placeholder "Filter PRs..." \
      --height 15 \
      --reverse); then
      return 1
    fi

    while IFS= read -r choice; do
      [ -z "$choice" ] && continue
      if [[ $choice =~ ^\[([0-9]+)\] ]]; then
        index=$((BASH_REMATCH[1] - 1))
      else
        echo "invalid selection: $choice" >&2
        return 1
      fi
      if [ "$index" -lt 0 ] || [ "$index" -ge "${#rows[@]}" ]; then
        echo "selection out of range: $((index + 1))" >&2
        return 1
      fi
      IFS=$'\t' read -r id repo number title url <<< "${rows[index]}"
      if [ -z "${seen[$id]+x}" ]; then
        selected+=("$id"$'\t'"$url")
        seen[$id]=1
      fi
    done <<< "$selection"

    [ "${#selected[@]}" -gt 0 ] && move_items "$target_number" "${selected[@]}"
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

show_clear_preview() {
    local mode=$1 id repo number title url state match colored_state
    CLEAR_IDS=()
    CLEAR_COUNT=0
    printf 'PRs to clear:\n'

    while IFS=$'\t' read -r id repo number title url; do
      if [ "$mode" = "all" ]; then
        match=1
        state=ALL
      else
        state=$(pr_state "$url")
        case "$mode" in
          closed)   [ "$state" = "CLOSED" ] && match=1 || match=0 ;;
          merged)   [ "$state" = "MERGED" ] && match=1 || match=0 ;;
          not-open) [ "$state" != "OPEN" ] && match=1 || match=0 ;;
          *) echo "unknown mode: $mode" >&2; return 1 ;;
        esac
      fi
      if [ "$match" = 1 ]; then
        if [ "$state" = "ALL" ]; then
          colored_state=$state
        else
          colored_state=$(status_label "$state")
        fi
        printf '  %-7s %s #%s  %s\n' "$colored_state" "$repo" "$number" "$title"
        CLEAR_IDS+=("$id")
        CLEAR_COUNT=$((CLEAR_COUNT + 1))
      fi
    done < <(items | jq -r '.items[] | select(.content.type=="PullRequest") |
      [.id, .content.repository, .content.number, .content.title, .content.url] | @tsv')

    if [ "$CLEAR_COUNT" -eq 0 ]; then
      printf '  (nothing)\n'
    fi
}

clear_interactive() {
    local mode=$1 prompt=$2
    show_clear_preview "$mode" || return 1
    [ "$CLEAR_COUNT" -gt 0 ] || return 1
    gum confirm "$prompt" || return 1
    remove_items "${CLEAR_IDS[@]}"
}

clear_secondary_interactive() {
    local original_number=$PROJECT_NUMBER original_name=${PROJECT_NAME:-} status
    select_secondary_project || return 1
    PROJECT_NUMBER=$SELECTED_SECONDARY_PROJECT_NUMBER
    PROJECT_NAME=$SELECTED_SECONDARY_PROJECT_NAME
    clear_interactive all "Clear all PRs from \"$PROJECT_NAME\"?"
    status=$?
    PROJECT_NUMBER=$original_number
    PROJECT_NAME=$original_name
    return "$status"
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
    local index=0 key rest i menu_text tracker_single="t  Add a PR to another project" tracker_multiple="T  Add multiple PRs to another project" move_option="m  Move PRs from \"$PROJECT_NAME\" to another project" tracker_heading="Another project"
    if [ -n "${SECONDARY_PROJECT_NUMBER:-}" ]; then
      tracker_single="t  Add a PR to \"$SECONDARY_PROJECT_NAME\""
      tracker_multiple="T  Add multiple PRs to \"$SECONDARY_PROJECT_NAME\""
      move_option="m  Move PRs from \"$PROJECT_NAME\" to \"$SECONDARY_PROJECT_NAME\""
      tracker_heading="$SECONDARY_PROJECT_NAME"
    fi
    local -a options=(
      "a  Add a PR"
      "A  Add multiple PRs"
      "c  Clear all closed and merged PRs"
      "r  Choose PR(s) to remove"
      "x  Clear all PRs"
      "l  List all PRs"
      "v  Open PR view"
      "p  Open project view"
      "$tracker_single"
      "$tracker_multiple"
      "$move_option"
      "R  Choose PR(s) to remove"
      "X  Clear all PRs"
    )

    while true; do
      menu_text="Manage PRs"
      menu_text+=$'\n\n'
      menu_text+="  "$'\033[4;38;5;215m'"$PROJECT_NAME"$'\033[0m\n'
      for i in "${!options[@]}"; do
        if [ "$i" -eq "$index" ]; then
          menu_text+=$'\033[38;5;212m❯ '
          menu_text+="${options[i]}"
          menu_text+=$'\033[0m\n'
        else
          menu_text+="  ${options[i]}"$'\n'
        fi
        case "$i" in
          1|4) menu_text+=$'\n' ;;
          7) menu_text+=$'\n  '$'\033[4;38;5;215m'"$tracker_heading"$'\033[0m\n' ;;
        esac
      done
      menu_text+=$'\n'
      menu_text+=$'\033[38;5;241m↑/↓ or j/k navigate · enter select · q/esc/ctrl-c quit\033[0m\n'
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
        a|A|t|T|m|M|R|X|c|C|r|x|l|L|v|V|p|P|q|Q) MENU_CHOICE=$key; return ;;
        ""|$'\n'|$'\r') MENU_CHOICE=${options[index]:0:1}; return ;;
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
    secondary_project_name
    interactive_menu
    printf '\033[H\033[2J'
    case "$MENU_CHOICE" in
      a)
        if add_command; then success_exit; fi
        ;;
      A)
        if add_multiple_command; then success_exit; fi
        ;;
      t)
        if add_to_tracker_command; then success_exit; fi
        ;;
      T)
        if add_multiple_to_tracker_command; then success_exit; fi
        ;;
      m|M)
        if choose_move_command; then success_exit; fi
        ;;
      R)
        if choose_remove_secondary_command; then success_exit; fi
        ;;
      X)
        if clear_secondary_interactive; then success_exit; fi
        ;;
      c|C)
        if clear_interactive not-open "Clear these non-open PRs?"; then success_exit; fi
        ;;
      r)
        if choose_remove_command; then success_exit; fi
        ;;
      x)
        if clear_interactive all "Clear all listed PRs?"; then success_exit; fi
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
