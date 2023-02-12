# ZAPPING

A retro-styled static HTML page to organize your YouTube playlists and video series. Think of it as channel surfing for the internet age.

Catppuccin Mocha theme. 90s aesthetic. CRT scanlines. No build step. Just open the file.

## Quick Start

1. Copy `playlists.example.js` to `playlists.js`
2. Open `index.html` in your browser

```
cp playlists.example.js playlists.js
firefox index.html
```

For quick access, set up a Firefox keyword bookmark pointing to `file:///path/to/index.html#watching`.

## Adding Playlists

### With the script

Requires [gum](https://github.com/charmbracelet/gum).

```
./add.sh
```

Paste a link, fill in the details, done. Supports auto-fetching metadata from YouTube and vivaplus.tv.

### Manually

Edit `playlists.js` and add an entry:

```js
{
  title: "Show Name",
  channel: "Channel Name",
  url: "https://www.youtube.com/playlist?list=PL...",
  thumb: "https://i.ytimg.com/vi/.../hqdefault.jpg",
  category: "Podcasts",
  status: "watching",
},
```

### Fields

| Field      | Required | Description                              |
|------------|----------|------------------------------------------|
| `title`    | yes      | Show name                                |
| `url`      | yes      | Link to playlist (any site)              |
| `channel`  | no       | Channel or creator name                  |
| `thumb`    | no       | Thumbnail URL (auto-detected for YT)     |
| `category` | no       | Group name (e.g. "Podcasts", "D&D")      |
| `status`   | no       | `watching`, `to-watch`, or `watched`     |

## Filtering

Use the filter bar or URL hash to jump to a view:

- `index.html#watching` -- currently watching
- `index.html#to-watch` -- watchlist
- `index.html#watched` -- finished
- `index.html#all` -- everything
