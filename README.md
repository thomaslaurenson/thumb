# thumb

![Release Build](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/thumb/tag.yml?style=flat&label=release&logo=github) ![Main Build](https://img.shields.io/github/actions/workflow/status/thomaslaurenson/thumb/main.yml?style=flat&label=main&logo=github)

![Release Version](https://img.shields.io/github/v/release/thomaslaurenson/thumb?style=flat&logo=github) ![Release downloads](https://img.shields.io/github/downloads/thomaslaurenson/thumb/total?style=flat&label=downloads&logo=github)

![Go Version](https://img.shields.io/github/go-mod/go-version/thomaslaurenson/thumb?style=flat&logo=go) ![Code Coverage](https://img.shields.io/badge/Coverage-87.0%25-blue?style=flat&logo=go)

Finger reads it, thumb writes it. `thumb` keeps a daily `.plan` file and a `project` list in the style of the 1990s finger plans, and publishes them as a small static site. It works in any terminal, including over SSH.

## Installation

```sh
curl -fsSL https://github.com/thomaslaurenson/thumb/releases/latest/download/install.sh | bash
```

Or from source: `go install github.com/thomaslaurenson/thumb@latest`.

## Data repository

thumb works on a folder holding a `thumb.conf`, found by walking up from the working directory, or named by `--dir` or `THUMB_DIR`, which must point at a folder that holds one:

```text
thumb.conf              title, author, login, base_url, description, timezone (key=value)
project                 one "KEYWORD description" per line
plans/2026/2026-10-24.plan
```

A plan line starts with a marker and a space; anything else is prose.

The site publishes entries up to today and holds back any dated later, such as an item migrated to tomorrow, until their day comes. Set `timezone` to your IANA zone, for example `timezone=Pacific/Auckland`, so a build on a CI runner agrees with you about which day it is; without it, thumb uses the clock of whichever machine runs the build.

| Marker | Meaning |
|---|---|
| `*` | Done that day |
| `+` | Done later, an older item resolved |
| `-` | To do |
| `~` | Dropped |
| `!` | Blocked or important |
| `?` | Idea or question |
| `>` | Moved to another day, written `> text -> YYYY-MM-DD` |

## The interface

Run `thumb` with no arguments. You browse and mark with vim keys, and type lines in place. Changes stay in memory until `ZZ`, which saves and quits; if the terminal closes first, thumb saves on the way out. Press `?` in the app for every key.

| Key | Action |
|---|---|
| `j` `k` or `down` `up`, `ctrl+d` `ctrl+u`, `gg` `G` | Move down or up, half a page, first or last line |
| `o` `O` | Add a line below or above and start typing |
| `i` `A` | Edit the line from the start of its text or from the end |
| `dd` | Delete the line |
| `space` | Mark done: `*` on today, `+` on an earlier day; again for to do |
| `r-` `r*` `r+` `r~` `r!` `r?` | Set that marker, as vim's `r` replaces a character |
| `r>` | Migrate an open item or idea to another day |
| `h` `l` or `left` `right`, `t`, `enter` | Previous or next day, today, open the selected day |
| `ctrl+a` or `ctrl+b`, then `n` `p` or `1`-`4` | Next or previous view, or day, open, archive, project |
| `/`, `esc` | Filter, clear the filter |
| `u` | Undo |
| `ZZ` | Save and quit |
| `?` | Every key |

The views are windows, switched with tmux's keys after either prefix: `ctrl+b` as in tmux, or `ctrl+a` as in GNU screen. Inside tmux, which keeps `ctrl+b` for itself, use `ctrl+a` (or press `ctrl+b` twice); if your own prefix is `ctrl+a`, use `ctrl+b`.

While typing, `enter` keeps the line and starts the next one with the same marker, so a list is typed one line after another; `enter` on a new line left untouched stops. `esc` or `ctrl+c` keeps the line and stops. Everything typed in one go is a single step for `u`, so `esc` then `u` takes it all back, and `esc` then `dd` removes just the last line.

`r>` opens a picker above the status bar, on tomorrow for a line on today and on today for any other. `h` `l` step a day and `j` `k` a week, `t` goes back to today, and `tab` `shift+tab` jump to the next or previous day with an entry; `i` types the day instead. The picker names the day and how far off it is, and shows its week with today in bold and days that have an entry underlined. `enter` migrates, `esc` cancels.

On the first run of a day, if today has no entry and the previous day has open items, thumb offers to migrate them, all at once or one by one.

## Commands

```sh
thumb todo write docs        # add "- write docs" to today
thumb done fixed rcon        # add "* fixed rcon"
thumb open                   # open items with DATE:LINE refs
thumb resolve 2026-10-24:4   # mark it "+"
thumb migrate 2026-10-24:4   # move it to today, leaving a ">" line
thumb list | show | project | check
thumb build                  # site into build/
thumb serve                  # preview at http://127.0.0.1:7979, reloading on save
thumb sync                   # commit plans and project, pull --rebase, push
thumb completion bash        # shell completion, including dates and refs
```

For `todo`, `done`, `idea` and `block`, flags go before the text: once the text starts, the rest of the line is text, flags included. Text that starts with a dash needs `--` first: `thumb idea -- -v for verbose`.

## Development

```sh
make help
make ci
```
