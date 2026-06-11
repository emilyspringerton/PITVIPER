# PITVIPER — North Star

**A standalone GPU-accelerated terminal emulator with Emily Prime integration hooks.**

> Not a TUI framework. Not a wrapper around an existing terminal. A purpose-built SDL2 terminal that
> renders at native GPU speed, embeds Emily Prime as a first-class process, and exposes every
> operator interaction as an auditable signal.

---

## Why Build a Terminal

Every agent interaction happens inside a terminal. The terminal is the last interface layer before
the human. If the terminal is someone else's code, Emily Prime's audit trail has a gap.

PITVIPER closes that gap:

| Problem | PITVIPER Solution |
|---|---|
| Terminal latency artifacts in dense signal feeds | SDL2 GPU rendering — no redraw latency |
| Multiplexer opacity (tmux/screen hide agent activity) | Native pane model with per-pane event hooks |
| No structured output from agent runs | Every command line produces a structured `CommandRecord` |
| Emily Prime lives outside the terminal | First-class `emily://` pane type, not a subprocess hack |
| Font rendering inconsistencies across distros | Embedded FreeType2, single font stack, zero system deps |

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   SDL2 Event Loop                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ Keyboard │  │  Mouse   │  │  Signal/Resize   │  │
│  └────┬─────┘  └────┬─────┘  └────────┬─────────┘  │
│       └─────────────┴─────────────────┘             │
│                    Dispatcher                        │
│  ┌──────────────────────────────────────────────┐   │
│  │               Pane Manager                   │   │
│  │  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │   │
│  │  │ Terminal │ │  Emily   │ │   Split /   │  │   │
│  │  │  Pane    │ │   Pane   │ │   Overlay   │  │   │
│  │  │ (pty)    │ │(emily://)│ │             │  │   │
│  │  └──────────┘ └──────────┘ └─────────────┘  │   │
│  └──────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────┐   │
│  │          Renderer (SDL2 + FreeType2)         │   │
│  │  GPU-accelerated glyph cache · 120fps target │   │
│  └──────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────┐   │
│  │           Emily Prime Hook Layer             │   │
│  │  CommandRecord · Apple POST · Obs watch      │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### Rendering Model

- **SDL2** surface per pane; composited to a single framebuffer each tick.
- **FreeType2** glyph rendering, embedded; no system font dependency at runtime.
- **Glyph cache**: LRU cache of pre-rendered glyph bitmaps, keyed by (codepoint, size, weight). Eliminates per-frame FreeType calls for hot glyphs.
- **Target**: 120fps on integrated Intel/AMD GPU; 60fps minimum on embedded ARM Mali.
- **Color space**: sRGB throughout; HDR path deferred to post-v1.

### Font Engine

- Primary font: **JetBrains Mono** (SIL OFL), embedded in binary as a Go `//go:embed` asset.
- Fallback chain: system monospace → embedded Noto Mono (CJK + symbol coverage).
- No Pango/HarfBuzz dependency; complex-script shaping via embedded HarfBuzz-subset.
- Font size and line height configurable via `config.toml`; live reload on SIGHUP.

### Multiplexer

- Native pane split model (horizontal/vertical, arbitrary depth).
- Each pane hosts exactly one **pane driver**: `pty`, `emily`, or `static` (log viewer).
- Pane layout serialized to `~/.local/share/pitviper/sessions/` on pane changes; restored on launch.
- No tmux compatibility layer — PITVIPER is not a drop-in replacement.

### `emily://` Pane

- Opens an embedded Emily Prime session (connects to `EMILY_AGENT_URL`, default `:8086`).
- Renders Emily's streaming responses with syntax highlighting and Apple citation links.
- Shortcut `Ctrl+E` opens/focuses the Emily pane from any pane.
- Observations written by Emily in the session are surfaced as inline pane notifications.

### Emily Prime Hook Layer

Every interactive event feeds the hook layer:

| Event | Hook | Destination |
|---|---|---|
| Command line submitted | `CommandRecord` JSON | `~/.local/share/pitviper/commands.ndjson` |
| Command exits with code | `ExitRecord` JSON | appended to same file |
| Emily observation fired | Apple POST | IDUNA `/api/v1/apples` |
| Pane opened/closed | `PaneEvent` JSON | same file |

`CommandRecord` schema:
```json
{
  "ts": "2026-06-11T09:00:00Z",
  "pane_id": "p0",
  "cmd": "go test ./...",
  "cwd": "/home/fatbaby/PRRJECT_FATBABY",
  "pid": 12345
}
```

Emily Prime reads `commands.ndjson` to understand what Emily Springerton is actively working on
— zero-latency context without clipboard polling or screen scraping.

---

## Milestones

### Milestone 0 — Bootstrap (SDL2 window, glyph rendering)
**Status:** `[ ] not started`

**Acceptance Criteria:**
- [ ] SDL2 window opens on Linux X11 and Wayland
- [ ] FreeType2 renders JetBrains Mono at 14pt into an SDL2 texture
- [ ] Backspace, Enter, printable ASCII input handled
- [ ] 60fps render loop with vsync; no CPU spin
- [ ] `pitviper --version` prints version string

### Milestone 1 — PTY Terminal Pane
**Status:** `[ ] queued`

**Acceptance Criteria:**
- [ ] PTY allocated via `openpty(3)`; shell ($SHELL or bash) launched inside
- [ ] VT100/xterm-256color escape sequence parser handles cursor movement, SGR colors, clear
- [ ] Scrollback buffer of 10,000 lines; scroll with Shift+PageUp/Down
- [ ] Resize propagated to PTY via TIOCSWINSZ
- [ ] `TERM=pitviper-256color` or `xterm-256color` set in child environment

### Milestone 2 — Glyph Cache + 120fps
**Status:** `[ ] queued`

**Acceptance Criteria:**
- [ ] LRU glyph cache: hit rate > 99% on typical shell session
- [ ] 120fps sustained on Intel Iris Xe (i7-1165G7 class)
- [ ] GPU memory usage < 64MB for a 1920×1080 session with 3 panes
- [ ] No tearing: SDL2 renderer swap interval 1

### Milestone 3 — Pane Splits + Session Persistence
**Status:** `[ ] queued`

**Acceptance Criteria:**
- [ ] Horizontal and vertical split via `Ctrl+W |` and `Ctrl+W -`
- [ ] Pane navigation: `Ctrl+W h/j/k/l` (vim-style)
- [ ] Layout serialized to JSON on every pane change
- [ ] Session restored on next launch (correct pane count, pane sizes, working dirs)
- [ ] `pitviper --session <name>` for named sessions

### Milestone 4 — Emily Prime Pane + Hook Layer
**Status:** `[ ] queued`

**Acceptance Criteria:**
- [ ] `emily://` pane connects to Emily Prime agent at `EMILY_AGENT_URL`
- [ ] Streaming response rendering with color-coded Apple citations
- [ ] `Ctrl+E` opens Emily pane from any active pane
- [ ] Every command line written to `commands.ndjson` as `CommandRecord`
- [ ] `ExitRecord` appended when command exits (exit code + duration)
- [ ] Emily observation in pane triggers Apple POST to IDUNA (best-effort)

### Milestone 5 — Font Engine Hardening
**Status:** `[ ] future`

- Noto Mono fallback for CJK + symbol glyphs
- HarfBuzz-subset for combining characters
- Ligature support (JetBrains Mono ligatures via OpenType GSUB)
- Font live-reload on SIGHUP

---

## Design Constraints (Never Compromise)

1. **No runtime system font dependency.** JetBrains Mono is embedded in the binary. Zero font setup on a fresh machine.
2. **SDL2 only.** No GTK, no Qt, no Electron. The binary links SDL2 and FreeType2 — nothing else.
3. **PTY, not pipes.** The shell runs inside a real PTY so readline, colors, and job control work correctly.
4. **Hook layer is best-effort.** A failed Apple POST never blocks a keypress. The terminal is always responsive.
5. **No tmux/screen dependency.** PITVIPER's pane model is native; it does not wrap another multiplexer.
6. **Emily pane is optional.** PITVIPER is a useful terminal without Emily. `EMILY_AGENT_URL` unset = no Emily pane, no errors.

---

## Config

`~/.config/pitviper/config.toml`:

```toml
[terminal]
font_size   = 14
line_height = 1.4
scrollback  = 10000
shell       = ""  # default: $SHELL or /bin/bash

[rendering]
target_fps  = 120
vsync       = true

[emily]
agent_url  = "http://localhost:8086"
apple_post = true  # post observation Apples to IDUNA

[hooks]
commands_log = "~/.local/share/pitviper/commands.ndjson"
```

---

## Build

```sh
# Dependencies (Ubuntu/Debian)
sudo apt install libsdl2-dev libfreetype-dev

# Build
go build ./cmd/pitviper

# Run
./pitviper
```

`CGO_ENABLED=1` required for SDL2 and FreeType2 bindings.

---

## Version

`v0.0 — 2026-06-11 — Emily Prime (initial northstar)`
