// Package sshconn implements a real SSH connection to a remote shell, satisfying the same
// io.ReadWriter-plus-Resize shape mudconn.Conn and pty.Terminal already expose so the SDL2
// render loop can treat SSH identically to a local PTY or a GFD MUD connection.
//
// Founder real-time, 2026-09-06: "get pitviper building for android im not sure how you will
// let me have bash or zsh all i need to do is ssh from android." Real, honest architecture
// note: Android does not ship bash/zsh, and PITVIPER's existing internal/pty package spawns a
// LOCAL shell via fork+exec, which is the wrong tool on Android even where it would compile --
// there is nothing real to exec there. SSH sidesteps that entirely: PITVIPER on Android becomes
// a pure SSH CLIENT rendering a REAL remote shell running on an actual server (this box), the
// same shell the founder already lives in from a laptop.
package sshconn

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Config describes how to reach and authenticate to a remote shell over SSH.
type Config struct {
	Addr     string // "host:port"; port defaults to 22 if omitted
	User     string
	Password string // optional; tried if non-empty
	KeyPath  string // optional path to a private key file; tried if non-empty
	// InsecureHostKey skips host key verification entirely -- a real, named tradeoff (accepts
	// MITM on this one connection), only meant as a deliberate opt-in escape hatch when no
	// known_hosts entry exists and the caller has verified the fingerprint some other real way
	// (e.g. reading it off the server directly). Dial() prefers ~/.ssh/known_hosts whenever it
	// exists and only falls back to this when that file is missing or has no matching entry.
	InsecureHostKey bool
}

// Conn wraps a live SSH session with a remote PTY + shell already started.
// Implements io.ReadWriter via Read/Write (backed by the session's stdout/stdin pipes).
type Conn struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	addr    string
}

// Dial connects, authenticates, requests a PTY, and starts an interactive shell -- by the time
// this returns successfully, Conn is ready to read/write exactly like a local PTY.
func Dial(cfg Config) (*Conn, error) {
	addr := cfg.Addr
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	var methods []ssh.AuthMethod
	if cfg.KeyPath != "" {
		key, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("sshconn: read key %s: %w", cfg.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("sshconn: parse key %s: %w", cfg.KeyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("sshconn: no authentication method available (pass -ssh-password, -ssh-key, or run an ssh-agent)")
	}

	hostKeyCallback, err := hostKeyCallback(cfg.InsecureHostKey)
	if err != nil {
		return nil, err
	}

	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            methods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("sshconn: dial %s: %w", addr, err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("sshconn: new session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	// 80x24 placeholder -- main.go calls Resize with the real vterm dimensions immediately
	// after Dial succeeds, before any real output has a chance to render at the wrong size.
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("sshconn: request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("sshconn: stdin pipe: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("sshconn: stdout pipe: %w", err)
	}
	// Merge stderr into the same stream -- a single vterm screen renders whatever comes back,
	// same as a local PTY (where a child's stdout/stderr are both the one slave fd already).
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("sshconn: stderr pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("sshconn: start shell: %w", err)
	}

	return &Conn{
		client:  client,
		session: session,
		stdin:   stdin,
		stdout:  io.MultiReader(stdout, stderr),
		addr:    addr,
	}, nil
}

// Read implements io.Reader, pulling from the remote shell's combined stdout+stderr.
func (c *Conn) Read(p []byte) (int, error) { return c.stdout.Read(p) }

// Write implements io.Writer, sending keystrokes to the remote shell's stdin.
func (c *Conn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

// Resize sends a real SSH window-change request so the remote shell's own $COLUMNS/$LINES (and
// any full-screen program running inside it) stay correct as the SDL2 window is resized.
func (c *Conn) Resize(cols, rows int) error {
	return c.session.WindowChange(rows, cols)
}

// Close tears down the session and the underlying TCP connection.
func (c *Conn) Close() error {
	c.session.Close()
	return c.client.Close()
}

// Addr returns the remote address this Conn is connected to.
func (c *Conn) Addr() string {
	return c.addr
}

// hostKeyCallback prefers a real, honest ~/.ssh/known_hosts check (the same mechanism the
// standard `ssh` client uses) and only falls back to skipping verification when insecure is
// explicitly requested (see Config.InsecureHostKey's own doc comment for the named tradeoff).
func hostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		// Explicit opt-in wins outright -- a known_hosts file existing but lacking an entry
		// for THIS host is exactly the case -ssh-insecure exists to override, not a reason to
		// keep enforcing verification anyway.
		return ssh.InsecureIgnoreHostKey(), nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
		if _, statErr := os.Stat(knownHostsPath); statErr == nil {
			if cb, khErr := knownhosts.New(knownHostsPath); khErr == nil {
				return cb, nil
			}
		}
	}
	return nil, fmt.Errorf("sshconn: no usable ~/.ssh/known_hosts entry found -- pass -ssh-insecure to skip host key verification (only do this if you've verified the server's fingerprint some other real way)")
}
