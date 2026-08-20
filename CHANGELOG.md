## 2026-08-20
- F11 toggle 加入真實 JetBrains Mono shiny font(疊在 OG 點陣圖字型上,SDL2_ttf 為基礎) (sess-20260820-0649-a3f19d93)
- docs/NORTHSTAR.md §7:「wandering oracle」核心原則(不要太早收緊 feedback loop),創辦人明確要求成為核心 pillar (sess-20260820-0649-a3f19d93)
- 修復 alternate-screen 離開時 scroll region 沒重設的真實 bug(clear 看起來沒作用);GPU 字形圖集取代逐像素繪圖;PTY 讀取迴圈加 panic recover;記錄 PITVIPER=VPS-as-IDE 的整體願景 (sess-20260820-0649-a3f19d93)
- 真實彩色 emoji 渲染(SDL2_ttf + Noto Color Emoji),阻塞在 sudo-queue/19,gofmt 乾淨、預期的 pkg-config 錯誤已確認 (sess-20260820-0649-a3f19d93)
- 自動尋找 Git for Windows 安裝路徑(不只依賴 PATH)、Photoshop 風格縮放快捷鍵(Ctrl+/-、Ctrl+0、Ctrl+滾輪) (sess-20260820-0649-a3f19d93)
- 修復 Windows shell 解析:略過 System32 的舊版 WSL bash.exe stub,避免在未啟用 WSL 的機器上誤判啟動 WSL (sess-20260820-0649-a3f19d93)

- Windows build passing (ConPTY PTY backend, internal/pty/pty_windows.go) + mouse-drag text selection/copy/paste (Ctrl+Shift+C/V, middle-click) + full README refresh (Windows build mechanics, complete keybinding reference, brief philosophy section). Verified live via Xvfb+XTest and confirmed via real CI run (both Linux and Windows jobs green, run 32343167493). (sess-20260820-0649-a3f19d93)

## 2026-08-14
- make install now works (GOWORK=off fix) and installs to ~/.local/bin; new Ctrl+Alt+I hotkey SSHes into iduna.farthq.com from plain PTY mode; first README.md. commit d9cce1a. (sess-20260813-2154-dda37e8b)

- Fixed real compile error (terminal.Close func()-vs-func()error mismatch); flagged CLAUDE.md's stale 'Milestone 0 not started' claim against real existing code (sess-20260813-2154-dda37e8b)

## 2026-06-25

- feat(ci): GitHub Actions CI workflow — test, CGO/SDL2 build, construct bundle (pending GitHub remote creation at S127-04)

## 2026-06-24
- feat: S127-05 district overlay pane — Ctrl+D, gfdapi.DistrictSnapshot, renderDistrictPane 20-col right pane (Apple #3652)
- feat: S127-02 Channel 11 splash screen — 2s gold logo + blinking CONNECTING on --gfd connect (Apple #3648)
- feat: S127-01 --gfd auto-login — GFD_USER+GFD_PASS env, detects 'Enter your name:' and password prompts in TCP stream (Apple #3646)
- feat: GFD SDL2 integration — --gfd TCP MUD mode, Channel 11 bar, --gfd-webmaster Emily gear overlay, gfdapi live state polling, mudconn TCP conn package (Apple #3508)

## 2026-06-21
- test: S67-01 font package test suite (Apple #2407)
- feat: S64-01 CSI E/F/X (CNL/CPL/ECH); 31 vterm tests (Apple #2400)
- feat: S63-01 CSI L/M insert/delete line; 30 vterm tests (Apple #2398)
- feat: S59-01 CSI S/T/P/@ scroll and char-edit operations; 29 vterm tests (Apple #2389)
- feat: S58-01 HT tab stop + TestTabStop — 27 vterm tests (Apple #2387)
- feat: S55-02 IsAltActive() + TestIsAltActive + TestSGRDefaultColors — 26 tests (Apple #2377)
- test: S55-01 TestEraseInDisplay 0J/1J — 24 vterm tests (Apple #2375)
- feat: S52-01 DECSTBM scroll region + ESC M reverse index + TestDECSTBM + TestReverseIndex (Apple #2361)
- feat: S51-02 alternate screen buffer ?1049h/?1049l + TestAlternateScreen (Apple #2358)
- feat: S51-01 cursor visibility ?25l/?25h + TestCursorVisibility (Apple #2355)
- feat: S47-01/02 OSC window title + snap-to-live on TextInput (Apple #2331)
- feat: S45-05 10,000-line scrollback buffer + Shift+PageUp/Down (Apple #2328)
- feat: S45-04 full SGR + cursor escape sequence parser, 14 vterm tests pass (Apple #2318)
- feat: S45-03 UTF-8 decode in vterm + ASCII glyph atlas pre-render (Apple #2315)

- feat: S45-02 PITVIPER Milestone 1 scaffold — vterm state machine (8 tests), PTY, glyph atlas, SDL2 main loop (Apple #2313)


## 2026-07-19
- feat(ci): add Windows build job to ci.yml — builds pitviper.exe via MSYS2/MinGW64 (SDL2 +
  SDL2_image + SDL2_ttf + matching gcc/Go toolchain, avoids GOROOT/gcc-ABI mismatches from mixing
  actions/setup-go with a separate mingw install), bundles required DLLs via ldd, uploads as its
  own artifact (pitviper-windows-<run>-<sha>). Same pattern as SHANKPIT's release.yml, adapted for
  Go+cgo instead of raw C.
