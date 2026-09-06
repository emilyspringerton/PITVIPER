// pitviper-enroll is a tiny, single-use HTTP listener that lets a freshly-installed PITVIPER
// (mobile, most concretely -- see cmd/pitviper's own -ssh-enroll flag) add its own newly
// generated SSH public key to this machine's ~/.ssh/authorized_keys, without anyone ever having
// to hand-type or hand-copy the key.
//
// Founder real-time, 2026-09-06, after "get pitviper building for android ... all i need to do
// is ssh from android" led into "how the fuck does the public key get onto the server? you gotz
// some magic its cool if u do?": real, deliberate design. Only the PUBLIC key ever crosses the
// network (safe by construction -- it reveals nothing useful to anyone else), and a random,
// single-use, short-lived enrollment code gates who's allowed to append one, so a stranger who
// happens to hit this port during the pairing window can't add their own key. This process
// exits after the first successful enrollment (or after a timeout) -- it is not a standing
// service, it's run by hand for the one minute it takes to pair a new device.
//
// Usage:
//
//	go run ./cmd/pitviper-enroll -addr :8099
//
// Prints a real, random 6-digit code to stdout, then waits for exactly one matching
// POST /enroll {"code":"123456","pubkey":"ssh-ed25519 AAAA... pitviper"} before appending that
// pubkey to ~/.ssh/authorized_keys (de-duplicated -- re-enrolling the same key is a no-op, not a
// duplicate line) and exiting 0. Exits 1 if no valid request arrives within -timeout.
package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type enrollRequest struct {
	Code   string `json:"code"`
	Pubkey string `json:"pubkey"`
}

func main() {
	addr := flag.String("addr", ":8099", "address to listen on for the one enrollment request")
	timeout := flag.Duration("timeout", 5*time.Minute, "how long to wait before giving up")
	flag.Parse()

	code, err := randomCode()
	if err != nil {
		log.Fatalf("pitviper-enroll: generate code: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("pitviper-enroll: home dir: %v", err)
	}
	authorizedKeysPath := filepath.Join(home, ".ssh", "authorized_keys")

	done := make(chan bool, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/enroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req enrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Code != code {
			// Real, deliberate: no hint about right/wrong beyond a flat 403 -- this is a
			// short-lived, single-purpose listener, not a login form that needs friendly errors.
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		pubkey := strings.TrimSpace(req.Pubkey)
		if pubkey == "" || !strings.HasPrefix(pubkey, "ssh-") {
			http.Error(w, "pubkey must be a real OpenSSH authorized_keys line", http.StatusBadRequest)
			return
		}
		if err := appendAuthorizedKey(authorizedKeysPath, pubkey); err != nil {
			log.Printf("pitviper-enroll: append key: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "enrolled")
		log.Printf("pitviper-enroll: enrolled a new key from %s: %s", r.RemoteAddr, pubkey)
		select {
		case done <- true:
		default:
		}
	})

	server := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("pitviper-enroll: listen: %v", err)
		}
	}()

	fmt.Println("Enrollment code (type this into PITVIPER's -ssh-enroll-code, or paste when prompted):")
	fmt.Println()
	fmt.Println("   " + code)
	fmt.Println()
	fmt.Printf("Listening on %s for one matching request (expires in %s)...\n", *addr, *timeout)

	select {
	case <-done:
		fmt.Println("Enrolled. Shutting down.")
		server.Close()
		os.Exit(0)
	case <-time.After(*timeout):
		fmt.Println("Timed out waiting for enrollment -- no key was added.")
		server.Close()
		os.Exit(1)
	}
}

func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func appendAuthorizedKey(path, pubkey string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pubkey {
			return nil // already present, nothing to do
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(pubkey + "\n")
	return err
}
