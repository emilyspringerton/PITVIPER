// Package mudconn implements a raw TCP connection to the GFD DragonsNShit MUD server.
// It satisfies the same io.ReadWriter interface that pty.Terminal exposes, so the
// SDL2 render loop can treat MUD and PTY identically.
package mudconn

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// Conn wraps a TCP connection to the MUD server.
// After Dial, reads from Master (the conn itself) and writes to Master.
// Implements io.ReadWriter via Master.
type Conn struct {
	Master net.Conn
	addr   string
	mu     sync.Mutex
	closed bool
}

// Dial opens a TCP connection to addr (e.g. "localhost:2323").
// Returns error immediately if the server is unreachable.
func Dial(addr string) (*Conn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("mudconn: dial %s: %w", addr, err)
	}
	return &Conn{Master: c, addr: addr}, nil
}

// Close tears down the TCP connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Master.Close()
}

// Resize is a no-op for TCP MUD connections (the server uses fixed-width text),
// present to satisfy the same interface as pty.Terminal.
func (c *Conn) Resize(cols, rows int) error {
	return nil
}

// Addr returns the remote address the Conn is connected to.
func (c *Conn) Addr() string {
	return c.addr
}
