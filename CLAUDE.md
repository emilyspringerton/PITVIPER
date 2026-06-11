# PITVIPER

PITVIPER is a standalone SDL2 terminal emulator with Emily Prime integration hooks.
Not a TUI framework — a purpose-built terminal that renders at native GPU speed and
exposes every operator interaction as an auditable signal.

## North Star

`docs/NORTHSTAR.md` — Milestone-gated delivery from SDL2 bootstrap to Emily Prime pane + hook layer.

## Stack

- Go 1.22+ with `CGO_ENABLED=1`
- SDL2 (graphics surface, event loop, vsync)
- FreeType2 (glyph rendering, embedded JetBrains Mono)
- PTY via `openpty(3)` + `TIOCSWINSZ`
- Emily Prime agent at `EMILY_AGENT_URL` (optional)

## Status

**Milestone 0 not started.** See `docs/NORTHSTAR.md` for full milestone plan.

## Related Repos

- `github.com/emilyspringerton/EMILY` — Emily Prime agent (`:8086`)
- `github.com/emilyspringerton/IDUNA` — IAM + Apples (Apple POST destination)
- `github.com/emilyspringerton/EmilyOS` — Policy kernel (PITVIPER is the operator interface)
