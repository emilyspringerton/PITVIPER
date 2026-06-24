// Package gfdapi provides lightweight hooks to GFD APIs for the PitViper SDL2 client.
//
// Used in webmaster mode to overlay live district state, Emily Prime gear,
// and IDUNA subscription tier onto the Channel 11 status bar.
//
// All calls are best-effort and fire in background goroutines — a failed API call
// never stalls the SDL2 render loop.
package gfdapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Config holds the GFD API connection settings.
type Config struct {
	IDUNABase   string // e.g. "http://localhost:8080"
	MUDAddr     string // e.g. "localhost:2323"
	Token       string // JWT from $GFD_TOKEN env var (webmaster token)
}

// State is the live overlay state polled from the GFD APIs.
type State struct {
	mu           sync.RWMutex
	EmilyGear    string    // "ACTIVE" | "COAST" | "REST" | "UNKNOWN"
	OnlineCount  int
	LatestApple  string    // title of the most recent Apple
	TierName     string    // current user's GFD tier (if JWT set)
	LastUpdated  time.Time
}

func (s *State) clone() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return State{
		EmilyGear:   s.EmilyGear,
		OnlineCount: s.OnlineCount,
		LatestApple: s.LatestApple,
		TierName:    s.TierName,
		LastUpdated: s.LastUpdated,
	}
}

// Client polls GFD APIs and maintains a live State.
type Client struct {
	cfg    Config
	state  State
	http   *http.Client
	stopCh chan struct{}
}

// NewClient creates a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 3 * time.Second},
		stopCh: make(chan struct{}),
	}
}

// Start launches background polling goroutines. Call once after NewClient.
func (c *Client) Start() {
	go c.pollLoop()
}

// Stop halts background polling.
func (c *Client) Stop() {
	close(c.stopCh)
}

// Snapshot returns a point-in-time copy of the live state.
func (c *Client) Snapshot() State {
	return c.state.clone()
}

func (c *Client) pollLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	c.poll() // immediate first poll
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.poll()
		}
	}
}

func (c *Client) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.LastUpdated = time.Now()

	// Poll Emily Prime health (gear state).
	if gear, err := c.fetchEmilyGear(ctx); err == nil {
		c.state.EmilyGear = gear
	} else {
		c.state.EmilyGear = "UNKNOWN"
	}

	// Poll latest Apple.
	if apple, err := c.fetchLatestApple(ctx); err == nil {
		c.state.LatestApple = apple
	}

	// Poll GFD user tier if token set.
	if c.cfg.Token != "" {
		if tier, err := c.fetchUserTier(ctx); err == nil {
			c.state.TierName = tier
		}
	}
}

func (c *Client) fetchEmilyGear(ctx context.Context) (string, error) {
	resp, err := c.get(ctx, "http://localhost:8086/health")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		return "", err
	}
	if gear, ok := m["gear"]; ok {
		return gear, nil
	}
	if m["ok"] == "true" || string(body) != "" {
		return "ACTIVE", nil
	}
	return "", fmt.Errorf("no gear field")
}

func (c *Client) fetchLatestApple(ctx context.Context) (string, error) {
	url := c.cfg.IDUNABase + "/api/v1/apples?limit=1"
	resp, err := c.getWithToken(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Apples []struct {
			Title string `json:"title"`
		} `json:"apples"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Apples) > 0 {
		return result.Apples[0].Title, nil
	}
	return "", nil
}

func (c *Client) fetchUserTier(ctx context.Context) (string, error) {
	url := c.cfg.IDUNABase + "/api/v1/subscriptions/me"
	resp, err := c.getWithToken(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", err
	}
	if tier, ok := m["gfd_tier"].(string); ok {
		return tier, nil
	}
	return "free_trial", nil
}

func (c *Client) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(req)
}

func (c *Client) getWithToken(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	return c.http.Do(req)
}
