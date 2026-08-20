# PITVIPER

PITVIPER is a standalone SDL2 terminal emulator with Emily Prime integration hooks.
Not a TUI framework — a purpose-built terminal that renders at native GPU speed and
exposes every operator interaction as an auditable signal.

**The real mission, in the founder's own words (2026-08-20)**: "basically i am extending my
IDE which is actually this VPS" — PITVIPER isn't just a terminal, it's the lightweight,
GPU-accelerated, low-latency client window into treating this VPS itself as the IDE. The
real work (editing, building, running) happens on the box; PITVIPER is how that stays fast
and usable from wherever the founder is. "like i am always in ssh" — the SSH-connected
session is the default, persistent mode, not something to route around; "i am using
pitviper to bring the affordances in a more gui way when we dont need to live in ssh
necessarily" — real GUI popouts (panels/widgets, not just VT100 text) layer *on top of*
that persistent SSH session for the cases where a pure text stream is the wrong fit, not a
separate mode. Concrete, real threads this mission is already driving: a vim-like editor
written in PARENA (SFTP + a NERDTree-style file-tree sidebar), and a custom PITVIPER server
component (AskUserQuestion-confirmed over adopting an existing protocol like Sixel) so the
GUI-popout affordance has something real to talk to — "lz4ify the fuck out of everything" /
"all the packet hacks we can" is the founder's own emphasis that this protocol has to stay
genuinely fast over a real network link, not just functionally correct. See
`EMILY/BACKLOG.md` S189-19+ for the live tracking; PARENA's own `NORTHSTAR.md` strangler-fig
adoption note is the mechanism this whole mission is threaded through.

## North Star

`docs/NORTHSTAR.md` — Milestone-gated delivery from SDL2 bootstrap to Emily Prime pane + hook layer.

## Stack

- Go 1.22+ with `CGO_ENABLED=1`
- SDL2 (graphics surface, event loop, vsync)
- FreeType2 (glyph rendering, embedded JetBrains Mono)
- PTY via `openpty(3)` + `TIOCSWINSZ`
- Emily Prime agent at `EMILY_AGENT_URL` (optional)

## Status

**Stale claim corrected 2026-08-14**: this used to say "Milestone 0 not started," but real code
already exists well past that (689-line `cmd/pitviper/main.go`, an 842-line `internal/vterm` with
555 lines of its own tests, real PTY handling, GFD API integration, MUD connection support) --
`docs/NORTHSTAR.md`'s own milestone checkboxes are similarly stale (still show Milestone 0-4 as
`not started`/`queued`). A real milestone-by-milestone status audit against actual code (which
acceptance criteria are genuinely met vs. still open) hasn't been done -- flagged, not done here.
See `docs/NORTHSTAR.md` for the full milestone plan (status markers unreliable until audited).

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
