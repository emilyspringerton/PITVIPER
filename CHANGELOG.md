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

