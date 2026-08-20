# PITVIPER

Standalone SDL2 terminal emulator with Emily Prime integration hooks — not a TUI framework,
a purpose-built terminal that renders at native GPU speed and doubles as the GoblinFoxDragon
(GFD) game client. See `docs/NORTHSTAR.md` for the full milestone plan and `CLAUDE.md` for
stack/status details.

## Why this exists

Every agent interaction happens inside a terminal, and the terminal is the last interface layer
before the human. If that terminal is someone else's code, there's a gap nothing else can see
into: font/latency quirks you can't fix, multiplexers that hide what's actually happening in a
pane, no structured record of what ran. PITVIPER is a from-scratch SDL2 terminal specifically so
that gap doesn't exist — GPU rendering instead of a redraw pipeline you don't control, and a
first-class integration point for Emily Prime instead of a subprocess hack bolted onto someone
else's emulator. Full rationale in `docs/NORTHSTAR.md`.

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

### Windows

PITVIPER also builds and runs natively on Windows — same source, same `cmd/pitviper` binary,
no WSL/Linux-subsystem requirement. Two things make that possible:

1. **A real Windows PTY backend.** POSIX systems give you one bidirectional PTY master fd
   (`internal/pty/pty_linux.go`, via `openpty(3)`). Windows has no such thing — the closest
   equivalent is **ConPTY** (`CreatePseudoConsole`), which hands you two *separate* pipes (one
   for keystrokes in, one for shell output out) instead of one fd. `internal/pty/pty_windows.go`
   wraps those two pipes behind the same `PTY.Master`/`.Resize()`/`.Close()` shape the Linux file
   exposes, so `cmd/pitviper/main.go` itself doesn't know or care which OS it's on — it just reads
   and writes an `io.Reader`/`io.Writer`. That's also why `main.go` only needs a `(linux ||
   windows) && cgo` build tag today, not per-OS code inside it.
2. **A real MinGW toolchain, not cross-compilation.** `go-sdl2` uses cgo, which means it needs
   an actual C compiler and SDL2 headers/libs for whatever OS you're targeting — you can't
   cross-compile a cgo binary for Windows from Linux without a full mingw-w64 toolchain sitting
   alongside a matching Go build (`sudo-queue/09-mingw-w64.sh` sets that up locally if you want
   to verify a Windows-shaped build on this box; it can't produce the SDL2-linked `.exe` itself
   without also vendoring MinGW's SDL2 libs, which is what CI does instead). CI's `build_windows`
   job (`.github/workflows/ci.yml`) sidesteps that entirely by running the **whole build**,
   compiler included, on a real `windows-latest` runner via MSYS2/MinGW64 — one Go, one gcc, one
   SDL2, all from the same toolchain, which is what `go-sdl2`'s cgo bridge actually needs to link
   cleanly. That job also bundles the SDL2 DLLs the resulting `.exe` dynamically links against
   plus a `RUN.bat` launcher into a downloadable CI artifact (`pitviper-windows-<run>-<sha>`) —
   grab that from the Actions run for a given commit, unzip, `RUN.bat`.

Why an `.exe` + DLLs + `.bat` and not one self-contained binary: `go-sdl2` dynamically links
SDL2 (and its own dependencies — SDL2_image, SDL2_ttf, libwinpthread, etc.) rather than
statically, which is the same tradeoff SHANKPIT's Windows release already makes. A fully static
single-binary build is possible in principle (MinGW has static SDL2 libs) but hasn't been built
or verified — flagged as a real future option, not attempted here.

## Usage

```sh
pitviper                              # plain terminal (PTY, $SHELL or /bin/bash)
pitviper --shell /bin/zsh             # explicit shell
pitviper --gfd localhost:2323         # connect as the GFD (GoblinFoxDragon) game client
pitviper --gfd localhost:2323 --gfd-webmaster   # + Emily Prime webmaster overlay
pitviper --version                    # print version and exit
```

On Windows, shell resolution (when `--shell` isn't passed) tries, in order: `$SHELL` env var →
`bash.exe`/`bash` on `PATH` (Git for Windows' own Git Bash — if it's installed, PITVIPER launches
straight into it, and Git Bash's own environment already resolves `ssh`, `~/.ssh`, and `HOME`
correctly with no special-casing on PITVIPER's side) → `cmd.exe` as the last-resort fallback.

## Driving the terminal — every keybinding, exactly what each does

| Key | Mode | What it does |
|---|---|---|
| *(typing)* | always | Goes straight to the child shell/MUD connection, same as any terminal |
| `Ctrl+<letter>` | always | Sent through as the real control character (`Ctrl+C` = SIGINT, `Ctrl+D` = EOF, etc. — standard shell semantics, PITVIPER doesn't intercept these) |
| `Shift+PageUp` | always | Scroll back half a screen into scrollback history |
| `Shift+PageDown` | always | Scroll forward half a screen |
| `Shift+Home` | always | Jump to the oldest scrollback line |
| `Shift+End` | always | Jump back to the live view |
| *(any other keypress while scrolled back)* | always | Snaps back to the live view automatically, like most terminals |
| `Ctrl+Alt+I` | plain PTY only | Types `ssh iduna.farthq.com` into the shell as if you'd typed it yourself — uses your real `~/.ssh/config`, keys, and `known_hosts`, no PITVIPER-side credential handling at all |
| `Ctrl+D` | `--gfd` only | Toggle the live district-state overlay pane |
| **Mouse** | | |
| Left-click + drag | always | Select text — highlighted live as you drag, copied to the clipboard automatically the moment you release the button (no extra keypress needed) |
| `Ctrl+Shift+C` | always | Explicitly re-copy the current selection to the clipboard (redundant with copy-on-release above, but there for the "I want to be sure" case — same binding PuTTY and most Linux terminals use) |
| `Ctrl+Shift+V` | always | Paste from the OS clipboard into the shell |
| Middle-click | always | Paste PITVIPER's own last selection (X11-primary-selection-style: select something, middle-click anywhere to drop it back in — **not the only way to paste**, `Ctrl+Shift+V` above is the primary, always-available path) |

**Why `Ctrl+Shift+C`/`V` and not plain `Ctrl+C`/`V`:** plain `Ctrl+C` is SIGINT and plain `Ctrl+V`
is a real shell control character (literal-next-char in readline) — both already have a job.
Adding `Shift` avoids colliding with either, same reasoning `Ctrl+Alt+I` already uses for the SSH
hotkey above.

**A real limitation, stated plainly, not glossed over:** middle-click paste here is PITVIPER's
*own* remembered last selection, not the operating system's primary-selection buffer. On Linux,
X11 has genuine cross-application primary selection (select text in one app, middle-click paste
it into a different one) — PITVIPER doesn't hook into that. On Windows there's no OS-level
equivalent to hook into at all. Keeping PITVIPER's own buffer means middle-click behaves
identically on both platforms, at the cost of only working with text selected inside PITVIPER
itself, not text copied from other applications (use `Ctrl+Shift+V` for that — it reads the real
OS clipboard on both platforms).

## Build & test

```sh
make build     # ./pitviper
make test      # go test ./internal/...
make install   # ~/.local/bin/pitviper
make deps      # sudo apt-get install libsdl2-dev (one-time)
```

`internal/pty` has no cgo dependency of its own (only `cmd/pitviper` does, for SDL2), so its
Windows file can be type-checked from Linux without a MinGW toolchain:
`GOWORK=off GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/pty/...`. That won't
catch SDL2/cgo-side Windows issues — CI's `build_windows` job (real `windows-latest` runner) is
the actual source of truth for whether a full Windows build passes.
