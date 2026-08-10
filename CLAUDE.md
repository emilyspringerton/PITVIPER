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

## Founder Real-Time Direction

Whenever the founder gives real-time direction — a new ask, a correction, a "can we also..." —
route it through `emily observe -s info "Founder real-time: <summary>"` first, even if it isn't
this repo's usual domain, then sprint-plan it into `EMILY/BACKLOG.md` (`emily backlog curate`,
scoped into a real SECTION/sub-item, not just a one-line log), and only then implement. See
`EMILY/docs/THE_EMILY_WAY.md` Principle 18 ("Pave the Cow Paths").

## Frame-Break Reframing

Founder-sourced prompting technique (REDGARDEN/NORTHSTAR.md §28, full origin in
REDGARDEN/docs2/MULTI_AGENT_RD_RESEARCH_NOTES.md §5): given a request, name the underlying
structural/systemic pattern it's one instance of — one level of abstraction up — as an added
lens during planning/triage/judgment calls. Use it to spot the general case behind a specific
ask. It augments judgment, it does not replace doing the work: direct, concrete execution of
the literal task asked for still happens every time.

## Commit Protocol (standing instruction)

Always commit and push completed work immediately — don't wait to be asked. This is the default for every repo in this monorepo.

Every commit — human-written or produced by automated code paths (git-commit helpers in emily-agent, emily.cli, IDUNA handlers, etc.) — must carry the active `emily session` fingerprint as a `session: <tag>` trailer (blank line, then the trailer). This was silently missing from several independently-implemented automated commit helpers across the monorepo until an audit on 2026-08-10 (founder, real-time: "where in the fuck is my llm session id anywhere"). If you add a new automated git-commit code path anywhere, wire in the session tag the same way — don't assume an existing helper already does it.
