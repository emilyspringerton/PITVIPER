# PITVIPER

Standalone SDL2 terminal emulator with Emily Prime integration hooks — not a TUI framework,
a purpose-built terminal that renders at native GPU speed and doubles as the GoblinFoxDragon
(GFD) game client. See `docs/NORTHSTAR.md` for the full milestone plan and `CLAUDE.md` for
stack/status details.

## Install

Same convention as `emily.cli` and IDUNA's CLI: installs to `~/.local/bin`, which is already
on `PATH` (see `~/.profile`) — no `sudo`, no `go install` (its default `$GOPATH/bin` isn't on
`PATH` here).

```sh
sudo apt-get install -y libsdl2-dev   # one-time, needs sudo (make deps)
make install                          # builds + installs to ~/.local/bin/pitviper
pitviper                              # runs as a bare command from any shell
```

PITVIPER is not part of the root `go.work` workspace — the Makefile always builds with
`GOWORK=off`. If you build manually, do the same:
`GOWORK=off CGO_ENABLED=1 go build -o pitviper ./cmd/pitviper`.

## Usage

```sh
pitviper                              # plain terminal (PTY, $SHELL or /bin/bash)
pitviper --shell /bin/zsh             # explicit shell
pitviper --gfd localhost:2323         # connect as the GFD (GoblinFoxDragon) game client
pitviper --gfd localhost:2323 --gfd-webmaster   # + Emily Prime webmaster overlay
```

## Keybindings

| Key | Mode | Action |
|---|---|---|
| `Ctrl+D` | `--gfd` only | Toggle the live district-state overlay pane |
| `Ctrl+Alt+I` | plain PTY only | Type `ssh iduna.farthq.com` into the shell — uses your normal `~/.ssh/config`/keys, no special-casing |

`Ctrl+Alt+I` rather than a plain `Ctrl+letter` deliberately — plain `Ctrl+<letter>` combos are
real shell control characters (Ctrl+C, Ctrl+D/EOF, readline bindings, etc.) and PITVIPER passes
those through to the child process untouched; only the district-overlay toggle carves out an
exception, and only inside `--gfd` mode where there's no real shell underneath to collide with.

## Build & test

```sh
make build     # ./pitviper
make test      # go test ./internal/...
make install   # ~/.local/bin/pitviper
make deps      # sudo apt-get install libsdl2-dev (one-time)
```
