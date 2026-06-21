//go:build linux

// Package pty manages a POSIX pseudo-terminal for the child shell process.
package pty

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// PTY wraps a master/slave PTY pair.
type PTY struct {
	Master *os.File
	Slave  *os.File
	Cmd    *exec.Cmd
}

// Open allocates a PTY and launches shell inside it with the given dimensions.
func Open(shell string, cols, rows int) (*PTY, error) {
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/bash"
	}

	master, slave, err := openPTY()
	if err != nil {
		return nil, fmt.Errorf("openpty: %w", err)
	}

	if err := setWinsize(master, cols, rows); err != nil {
		master.Close()
		slave.Close()
		return nil, err
	}

	cmd := exec.Command(shell)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		fmt.Sprintf("COLUMNS=%d", cols),
		fmt.Sprintf("LINES=%d", rows),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    3, // slave is fd 3 in child after stdin/stdout/stderr
	}
	// Attach slave as extra fd so Ctty index 3 is correct.
	cmd.ExtraFiles = []*os.File{slave}

	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	// Slave fd is no longer needed in parent after child started.
	slave.Close()

	return &PTY{Master: master, Cmd: cmd}, nil
}

// Resize notifies the PTY of a new terminal size.
func (p *PTY) Resize(cols, rows int) error {
	return setWinsize(p.Master, cols, rows)
}

// Close terminates the child process and closes the master fd.
func (p *PTY) Close() {
	if p.Cmd != nil && p.Cmd.Process != nil {
		_ = p.Cmd.Process.Kill()
		_ = p.Cmd.Wait()
	}
	if p.Master != nil {
		_ = p.Master.Close()
	}
}

func openPTY() (master, slave *os.File, err error) {
	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	master = os.NewFile(uintptr(masterFd), "/dev/ptmx")

	// Grant + unlock slave.
	if err := grantpt(masterFd); err != nil {
		master.Close()
		return nil, nil, err
	}
	if err := unlockpt(masterFd); err != nil {
		master.Close()
		return nil, nil, err
	}

	slaveName, err := ptsname(masterFd)
	if err != nil {
		master.Close()
		return nil, nil, err
	}

	slaveFd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("open slave %s: %w", slaveName, err)
	}
	slave = os.NewFile(uintptr(slaveFd), slaveName)
	return master, slave, nil
}

func grantpt(fd int) error {
	// On Linux with devpts, grantpt is a no-op.
	return nil
}

func unlockpt(fd int) error {
	n := 0
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCSPTLCK), uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return fmt.Errorf("TIOCSPTLCK: %w", errno)
	}
	return nil
}

func ptsname(fd int) (string, error) {
	var n uint32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCGPTN), uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return "", fmt.Errorf("TIOCGPTN: %w", errno)
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

type winsize struct {
	Row, Col       uint16
	Xpixel, Ypixel uint16
}

func setWinsize(f *os.File, cols, rows int) error {
	ws := winsize{Row: uint16(rows), Col: uint16(cols)}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(unix.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return fmt.Errorf("TIOCSWINSZ: %w", errno)
	}
	return nil
}
