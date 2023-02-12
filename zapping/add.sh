#!/usr/bin/env bash
set -euo pipefail

if ! command -v gum &>/dev/null; then
  echo "This script requires 'gum' (charmbracelet/gum)."
  echo ""
  echo "Install it with:"
  echo "  pacman -S gum        # Arch"
  echo "  brew install gum     # macOS"
  echo "  go install github.com/charmbracelet/gum@latest"
  echo ""
  echo "Then re-run this script."
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLAYLISTS_FILE="$HOME/.config/toolshed/zapping/playlists.js"
mkdir -p "$(dirname "$PLAYLISTS_FILE")"

# Catppuccin Mocha
CTP_MAUVE="203"
CTP_GREEN="10"
CTP_YELLOW="11"
CTP_RED="9"
CTP_SKY="14"

# ═══ Header ═══
gum style \
  --foreground "$CTP_MAUVE" --bold \
  --border double --border-foreground "$CTP_MAUVE" \
  --padding "0 2" --margin "1 0" \
  "ADD NEW PLAYLIST"

# ═══ URL ═══
URL=$(gum input --placeholder "Paste a link..." --header "Link:" --width 80)
if [[ -z "$URL" ]]; then
  gum style --foreground "$CTP_RED" "No URL provided."
  exit 1
fi

# ═══ Fetch metadata ═══
META_TITLE=""
META_CHANNEL=""
META_THUMB=""

if [[ "$URL" =~ youtube\.com|youtu\.be ]]; then
  # Convert /show/VL... URLs to regular playlist URLs for oEmbed
  OEMBED_URL="$URL"
  if [[ "$URL" =~ youtube\.com/show/VL([A-Za-z0-9_-]+) ]]; then
    OEMBED_URL="https://www.youtube.com/playlist?list=${BASH_REMATCH[1]}"
  fi
  gum spin --spinner dot --title "Fetching from YouTube..." -- bash -c '
    json=$(curl -sf "https://www.youtube.com/oembed?url='"$(python3 -c "import urllib.parse; print(urllib.parse.quote('$OEMBED_URL', safe=''))")"'&format=json" 2>/dev/null)
    if [[ -n "$json" ]]; then
      echo "$json" > /tmp/_add_playlist_meta.json
    fi
  '
  if [[ -f /tmp/_add_playlist_meta.json ]]; then
    META_TITLE=$(python3 -c "import json; d=json.load(open('/tmp/_add_playlist_meta.json')); print(d.get('title',''))" 2>/dev/null || true)
    META_CHANNEL=$(python3 -c "import json; d=json.load(open('/tmp/_add_playlist_meta.json')); print(d.get('author_name',''))" 2>/dev/null || true)
    META_THUMB=$(python3 -c "import json; d=json.load(open('/tmp/_add_playlist_meta.json')); print(d.get('thumbnail_url',''))" 2>/dev/null || true)
    rm -f /tmp/_add_playlist_meta.json
  fi
elif [[ "$URL" =~ vivaplus\.tv ]]; then
  gum spin --spinner dot --title "Fetching from Viva+..." -- bash -c '
    curl -sfL --compressed "'"$URL"'" > /tmp/_add_playlist_meta.html 2>/dev/null
  '
  if [[ -f /tmp/_add_playlist_meta.html ]]; then
    META_TITLE=$(grep -oP '<title>\K[^<]+' /tmp/_add_playlist_meta.html | sed 's/ *|.*//;s/^ *//;s/ *$//' | head -1 || true)
    META_CHANNEL="Viva la Dirt League"
    META_THUMB=$(grep -oP 'property="og:image"[^>]*content="\K[^"]+' /tmp/_add_playlist_meta.html | head -1 || true)
    if [[ -z "$META_THUMB" ]]; then
      META_THUMB=$(grep -oP 'content="\K[^"]+(?="[^>]*property="og:image")' /tmp/_add_playlist_meta.html | head -1 || true)
    fi
    rm -f /tmp/_add_playlist_meta.html
  fi
else
  gum style --foreground "$CTP_YELLOW" "Unknown source -- fill in details manually"
fi

if [[ -n "$META_TITLE" ]]; then
  gum style --foreground "$CTP_GREEN" "Got: $META_TITLE"
fi

# ═══ Confirm/override fields ═══
TITLE=$(gum input --header "Title:" --value "$META_TITLE" --width 80)
CHANNEL=$(gum input --header "Channel:" --value "$META_CHANNEL" --width 80)
THUMB=$(gum input --header "Thumbnail URL:" --value "$META_THUMB" --width 80)

# ═══ Status ═══
STATUS=$(gum choose --header "Status:" "watching" "to-watch" "watched")

# ═══ Category ═══
existing_cats=()
while IFS= read -r cat; do
  [[ -n "$cat" ]] && existing_cats+=("$cat")
done < <(grep -oP 'category:\s*"?\K[^",]+' "$PLAYLISTS_FILE" 2>/dev/null | sort -u)

cat_options=("${existing_cats[@]}" "+ New category")
CAT_CHOICE=$(gum choose --header "Category:" "${cat_options[@]}")

if [[ "$CAT_CHOICE" == "+ New category" ]]; then
  CATEGORY=$(gum input --header "New category name:" --width 40)
else
  CATEGORY="$CAT_CHOICE"
fi

# ═══ Build JS entry ═══
json_str() { python3 -c "import json,sys; print(json.dumps(sys.argv[1]))" "$1" 2>/dev/null || echo "\"$1\""; }

ENTRY="  {"
ENTRY+=$'\n'"    title: $(json_str "$TITLE"),"
[[ -n "$CHANNEL" ]] && ENTRY+=$'\n'"    channel: $(json_str "$CHANNEL"),"
ENTRY+=$'\n'"    url: $(json_str "$URL"),"
[[ -n "$THUMB" ]] && ENTRY+=$'\n'"    thumb: $(json_str "$THUMB"),"
[[ -n "$CATEGORY" ]] && ENTRY+=$'\n'"    category: $(json_str "$CATEGORY"),"
ENTRY+=$'\n'"    status: \"${STATUS}\","
ENTRY+=$'\n'"  },"

# ═══ Prepend to playlists.js ═══
TMPFILE=$(mktemp)
{
  head -1 "$PLAYLISTS_FILE"
  echo "$ENTRY"
  tail -n +2 "$PLAYLISTS_FILE"
} > "$TMPFILE"
mv "$TMPFILE" "$PLAYLISTS_FILE"

# ═══ Confirmation ═══
echo ""
gum style --foreground "$CTP_GREEN" --bold "Added!"
echo ""
echo "  Title:    $TITLE"
[[ -n "$CHANNEL" ]] && echo "  Channel:  $CHANNEL"
[[ -n "$CATEGORY" ]] && echo "  Category: $CATEGORY"
echo "  Status:   $STATUS"
echo "  URL:      $URL"
echo ""
gum style --faint "Reload the page to see changes."
