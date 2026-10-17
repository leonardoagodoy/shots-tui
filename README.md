# shots-tui

A terminal UI to browse and manage screenshots, with inline image preview.
Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and the
[kitty graphics protocol](https://sw.kovidgoyal.net/kitty/graphics-protocol/).

- Preview of the selected image next to the list (or fullscreen)
- Search, multi-select, rename, copy image to clipboard, move to trash
- Default folder: `$SHOTS_DIR` or `~/Pictures/Screenshots`

> The interface text is in Portuguese.

## Requirements

- A terminal that supports the kitty graphics protocol ([kitty](https://sw.kovidgoyal.net/kitty/), WezTerm, Ghostty…)
- `wl-copy` (Wayland) or `xclip` (X11) to copy images
- `gio` (glib) to move files to the trash, `xdg-open` to open files
- Go to build

## Install

```sh
git clone https://github.com/leonardoagodoy/shots-tui
cd shots-tui
go build -o shots .
./shots [directory]
```

## Keys

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | move |
| `enter` | open with `xdg-open` |
| `ctrl+f` | fullscreen preview |
| `ctrl+y` | copy image to clipboard |
| `F2` | rename |
| `ctrl+d` | move to trash |
| `tab` | mark / unmark |
| `ctrl+p` | toggle preview |
| `/` | search (`esc` closes) |
| `q` | quit |

## License

MIT
