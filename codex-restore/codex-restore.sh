#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  restore-codex-session.sh --backup-dir BACKUP_DIR --repo OWNER/REPO [OPTIONS]

Find a Codex session associated with OWNER/REPO in BACKUP_DIR, let the user
select it with fzf, restore only that session into CODEX_HOME, and resume it.

Arguments:
  --backup-dir DIR   Old Codex home, containing sessions/ and state_*.sqlite
  --repo OWNER/REPO  Repository identifier; inferred from origin when omitted

Options:
  --current-repo-dir DIR  Current checkout (default: current directory)
  -h, --help, help        Show this help

Examples:
  restore-codex-session.sh \
    --backup-dir /mnt/old/.codex \
    --repo openshift/enhancements
  restore-codex-session.sh \
    --backup-dir /mnt/old/.codex \
    --repo openshift/enhancements \
    --current-repo-dir /home/me/src/enhancements

The backup is not modified. Credentials are not copied; the selected session
transcript and its thread metadata are imported. With the default restore
location, plain `codex resume` will find the session afterward.
EOF
}

if [[ $# -eq 0 || "$1" == "-h" || "$1" == "--help" || "$1" == "help" ]]; then
  usage
  exit 0
fi

backup_dir=''
repo=''
current_repo=$PWD

while [[ $# -gt 0 ]]; do
  case "$1" in
    --backup-dir|--repo|--current-repo-dir)
      [[ $# -ge 2 ]] || { echo "Missing value for $1" >&2; exit 2; }
      case "$1" in
        --backup-dir) backup_dir=$2 ;;
        --repo) repo=$2 ;;
        --current-repo-dir) current_repo=$2 ;;
      esac
      shift 2
      ;;
    -h|--help|help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[[ -n "$backup_dir" ]] || {
  echo "--backup-dir is required" >&2
  usage >&2
  exit 2
}

[[ -d "$backup_dir" ]] || { echo "Backup directory does not exist: $backup_dir" >&2; exit 1; }
[[ -d "$current_repo" ]] || { echo "Current repository does not exist: $current_repo" >&2; exit 1; }

if [[ -z "$repo" ]]; then
  remote_url=$(git -C "$current_repo" remote get-url origin 2>/dev/null) || {
    echo "Could not determine repository: no origin remote in $current_repo" >&2
    exit 1
  }

  if [[ "$remote_url" =~ ^[^:/]+://[^/]+/(.+)$ ]]; then
    repo=${BASH_REMATCH[1]}
  elif [[ "$remote_url" =~ ^[^/:]+:([^/]+/.+)$ ]]; then
    repo=${BASH_REMATCH[1]}
  else
    echo "Could not parse origin remote URL: $remote_url" >&2
    exit 1
  fi

  repo=${repo%.git}
fi

[[ "$repo" =~ ^[^/]+/[^/]+$ ]] || {
  echo "Repository must have the form owner/repo: $repo" >&2
  exit 2
}

state_db=$(find "$backup_dir" -maxdepth 1 -type f -name 'state_*.sqlite' -print -quit)
[[ -n "$state_db" ]] || {
  echo "Could not find state_*.sqlite in $backup_dir" >&2
  exit 1
}

for command in sqlite3 fzf codex; do
  command -v "$command" >/dev/null || {
    echo "Required command not found: $command" >&2
    exit 1
  }
done

repo_sql=${repo//\'/\'\'}
repo_root="/redhat/repos/$repo_sql"

rows=$(sqlite3 -separator '|' "$state_db" "
  SELECT
    id,
    datetime(updated_at, 'unixepoch'),
    replace(replace(replace(title, char(9), ' '), char(10), ' '), char(13), ' '),
    cwd
  FROM threads
  WHERE cwd LIKE '%$repo_root%'
  ORDER BY updated_at DESC;
")

[[ -n "$rows" ]] || {
  echo "No sessions found for $repo" >&2
  exit 1
}

selected=$(printf '%s\n' "$rows" | fzf \
  --delimiter='|' \
  --with-nth=2.. \
  --header='Select a Codex session (ESC cancels)' \
  --height=40%) || {
    echo "No session selected." >&2
    exit 1
  }

session_id=${selected%%|*}
session_id_sql=${session_id//\'/\'\'}
session_file=$(find "$backup_dir/sessions" -type f -name "*-$session_id.jsonl" -print -quit)
[[ -n "$session_file" ]] || {
  echo "Could not find transcript for session $session_id" >&2
  exit 1
}

restore_dir=${CODEX_HOME:-"$HOME/.codex"}

relative=${session_file#"$backup_dir/"}
mkdir -p "$restore_dir/$(dirname "$relative")"
cp -- "$session_file" "$restore_dir/$relative"
restored_rollout_path="$restore_dir/$relative"

if [[ ! -e "$restore_dir/config.toml" && -f "$backup_dir/config.toml" ]]; then
  cp -- "$backup_dir/config.toml" "$restore_dir/config.toml"
fi

restored_state=$(find "$restore_dir" -maxdepth 1 -type f -name 'state_*.sqlite' -print -quit)
[[ -n "$restored_state" ]] || {
  echo "Could not find the destination Codex state database in $restore_dir" >&2
  exit 1
}

thread_columns=$(sqlite3 "$state_db" \
  "SELECT group_concat(char(34) || replace(name, char(34), char(34) || char(34)) || char(34), ',') FROM pragma_table_info('threads');")
thread_values=$(sqlite3 "$state_db" \
  "SELECT group_concat('quote(' || char(34) || replace(name, char(34), char(34) || char(34)) || char(34) || ')', ' || char(44) || ') FROM pragma_table_info('threads');")
[[ -n "$thread_columns" && -n "$thread_values" ]] || {
  echo "Could not inspect the backup threads table" >&2
  exit 1
}

thread_insert=$(sqlite3 "$state_db" \
  "SELECT 'INSERT OR IGNORE INTO threads ($thread_columns) VALUES (' || $thread_values || ');' FROM threads WHERE id = '$session_id_sql';")
[[ -n "$thread_insert" ]] || {
  echo "Could not find SQLite metadata for session $session_id" >&2
  exit 1
}

sqlite3 "$restored_state" <<SQL
PRAGMA foreign_keys = OFF;
$thread_insert
SQL

restored_rollout_path_sql=${restored_rollout_path//\'/\'\'}
current_repo_sql=${current_repo//\'/\'\'}
sqlite3 "$restored_state" \
  "UPDATE threads SET rollout_path = '$restored_rollout_path_sql', cwd = '$current_repo_sql' WHERE id = '$session_id_sql';"

echo "Restoring $session_id into $restore_dir" >&2
CODEX_HOME="$restore_dir" codex migrate-rollouts --apply

exec env CODEX_HOME="$restore_dir" codex -C "$current_repo" resume "$session_id"
