//go:build windows

// Package pty manages a Windows pseudo console (ConPTY) for the child shell process.
//
// ConPTY (unlike a POSIX PTY master fd) exposes input and output as two separate
// pipes rather than one bidirectional fd, so PTY.Master here is a small duplexPipe
// wrapper rather than a bare *os.File — it satisfies the same io.Reader/io.Writer
// contract main.go actually depends on (it only ever uses Master through those two
// interfaces, never as a concrete *os.File), so this stays a drop-in match for the
// Linux PTY's public shape without changing main.go.
package pty

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isWslStub reports whether path is the legacy %SystemRoot%\System32\bash.exe
// launcher, which every Windows install ships regardless of whether WSL is
// actually provisioned — exec.LookPath("bash.exe") happily resolves to it
// when Git Bash isn't installed or isn't earlier on PATH, and launching it
// without WSL enabled is exactly the "tries to launch Windows Subsystem for
// Linux and I don't have those bits turned on" failure (founder, 2026-08-20).
func isWslStub(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, `\system32\bash.exe`)
}

// findGitBash checks the well-known Git-for-Windows install locations
// directly, for the real case PATH-only lookup misses: Git installed but
// its own bin/ dir never added to PATH (the installer's default is
// actually to skip PATH modification for bash.exe specifically, only
// git.exe gets added) — founder, real-time: "can you make it
// automatically find gitbash on windows or something?" Checks both the
// system-wide and per-user (non-admin) install locations, 64-bit and
// 32-bit Program Files.
func findGitBash() string {
	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LocalAppData"), "Programs"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, sub := range []string{`Git\bin\bash.exe`, `Git\usr\bin\bash.exe`} {
			candidate := filepath.Join(root, sub)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return ""
}

// duplexPipe presents ConPTY's separate input/output pipe handles as one
// io.Reader + io.Writer, matching how main.go uses PTY.Master.
type duplexPipe struct {
	r *os.File // ConPTY's output pipe, read end — shell output arrives here
	w *os.File // ConPTY's input pipe, write end — keystrokes go here
}

func (d *duplexPipe) Read(p []byte) (int, error)  { return d.r.Read(p) }
func (d *duplexPipe) Write(p []byte) (int, error) { return d.w.Write(p) }

// WriteString avoids a []byte conversion for the common TextInputEvent path —
// same optional-interface optimization main.go already checks for on Linux's
// *os.File (which has WriteString natively).
func (d *duplexPipe) WriteString(s string) (int, error) { return d.w.WriteString(s) }

// PTY wraps a ConPTY pseudo console and the child shell process.
type PTY struct {
	Master *duplexPipe

	console  windows.Handle
	attrList *windows.ProcThreadAttributeListContainer
	proc     windows.Handle
	thread   windows.Handle

	// ConPTY's own ends of the pipes — closed once ownership passes to the
	// pseudo console at CreatePseudoConsole time, kept only to know what to
	// close on that handoff.
}

// Open allocates a ConPTY and launches shell inside it with the given dimensions.
// shell resolution order: explicit arg > $SHELL > Git Bash (bash.exe on PATH —
// "git bash figures it out" per founder direction, 2026-08-20: Git for Windows'
// own bash.exe already resolves ~/.ssh, ssh.exe, and HOME correctly with no
// PITVIPER-side special-casing) > cmd.exe as the last-resort fallback. The
// legacy System32\bash.exe WSL launcher stub is explicitly skipped (see
// isWslStub) — it's on PATH on stock Windows installs whether or not WSL is
// actually enabled, so an unguarded LookPath silently launches WSL instead
// of a real shell for users without it turned on. If PATH lookup finds
// nothing real, findGitBash() checks Git for Windows' own well-known
// install locations directly (its installer does not reliably add bin/ to
// PATH), before falling all the way back to cmd.exe.
func Open(shell string, cols, rows int) (*PTY, error) {
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		if p, err := exec.LookPath("bash.exe"); err == nil && !isWslStub(p) {
			shell = p
		}
	}
	if shell == "" {
		if p, err := exec.LookPath("bash"); err == nil && !isWslStub(p) {
			shell = p
		}
	}
	if shell == "" {
		shell = findGitBash()
	}
	if shell == "" {
		shell = "cmd.exe"
	}

	// Pipe 1: our write end feeds ConPTY's stdin (inRead is ConPTY's copy).
	var inRead, inWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	// Pipe 2: ConPTY's stdout (outWrite is ConPTY's copy) feeds our read end.
	var outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	var console windows.Handle
	size := windows.Coord{X: int16(cols), Y: int16(rows)}
	if err := windows.CreatePseudoConsole(size, inRead, outWrite, 0, &console); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		windows.CloseHandle(outWrite)
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	// ConPTY now owns these ends; our copies would otherwise keep the pipe
	// alive after the child exits and wedge the read/write side that's left.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(console),
		unsafe.Sizeof(console),
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, fmt.Errorf("attrList.Update: %w", err)
	}

	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	si.ProcThreadAttributeList = attrList.List()

	var pi windows.ProcessInformation
	cmdLine, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, fmt.Errorf("shell command line: %w", err)
	}

	err = windows.CreateProcess(
		nil, cmdLine, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, nil, &si.StartupInfo, &pi,
	)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(console)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, fmt.Errorf("CreateProcess %s: %w", shell, err)
	}
	// The thread handle isn't needed once the process is running.
	windows.CloseHandle(pi.Thread)

	master := &duplexPipe{
		r: os.NewFile(uintptr(outRead), "pitviper-conpty-out"),
		w: os.NewFile(uintptr(inWrite), "pitviper-conpty-in"),
	}

	return &PTY{
		Master:   master,
		console:  console,
		attrList: attrList,
		proc:     pi.Process,
		thread:   pi.Thread,
	}, nil
}

// Resize notifies the ConPTY of a new terminal size.
func (p *PTY) Resize(cols, rows int) error {
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}

// Close terminates the child process, tears down the pseudo console, and
// closes the pipe handles. Mirrors the Linux PTY's best-effort, no-error-return
// shape — main.go only ever calls this in a deferred cleanup.
func (p *PTY) Close() {
	if p.proc != 0 {
		_ = windows.TerminateProcess(p.proc, 0)
		_, _ = windows.WaitForSingleObject(p.proc, 2000)
		windows.CloseHandle(p.proc)
	}
	if p.attrList != nil {
		p.attrList.Delete()
	}
	if p.console != 0 {
		windows.ClosePseudoConsole(p.console)
	}
	if p.Master != nil {
		_ = p.Master.r.Close()
		_ = p.Master.w.Close()
	}
}
