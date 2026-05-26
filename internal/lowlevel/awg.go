package lowlevel

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────
// AWG obfuscation parameters
// ─────────────────────────────────────────────────────────────

// AWGParams holds AmneziaWG DPI-obfuscation parameters (version 1.x).
// All zero values = vanilla WireGuard (backward compatible).
type AWGParams struct {
	Jc         int    // junk packets before handshake [0–10]
	Jmin, Jmax int    // junk size range in bytes
	S1, S2     int    // random prefix bytes in handshake messages
	H1, H2, H3, H4 uint32 // replaced message types, pairwise unique
}

// IsZero returns true when all fields are at their zero value —
// amneziawg-go then behaves as vanilla WireGuard.
func (p AWGParams) IsZero() bool {
	return p.Jc == 0 && p.H1 == 0 && p.H2 == 0 && p.H3 == 0 && p.H4 == 0
}

// TODO(v2): extend with S3, S4, I1–I5 for CPS (QUIC/DNS mimic) when
// upgrading to amneziawg-go 2.0 protocol.

// ─────────────────────────────────────────────────────────────
// Process management — wg-portal manages amneziawg-go itself
// ─────────────────────────────────────────────────────────────

var (
	mu       sync.Mutex
	procs    = make(map[string]*exec.Cmd) // ifaceName → Cmd
	sockDir  = "/var/run/amneziawg"       // amneziawg-go hardcodes this
)

func socketPath(ifaceName string) string {
	return filepath.Join(sockDir, ifaceName+".sock")
}

// wgctrlSocketPath returns the path where wgctrl expects the socket.
func wgctrlSocketPath(ifaceName string) string {
	return filepath.Join("/var/run/wireguard", ifaceName+".sock")
}

// ensureSymlink creates /var/run/wireguard/<iface>.sock → /var/run/amneziawg/<iface>.sock
// so that wgctrl (which scans /var/run/wireguard/) can find the amneziawg socket.
func ensureSymlink(ifaceName string) error {
	target := socketPath(ifaceName)
	link := wgctrlSocketPath(ifaceName)

	// Remove stale symlink if any
	if _, err := os.Lstat(link); err == nil {
		os.Remove(link)
	}

	// Ensure /var/run/wireguard dir exists
	os.MkdirAll(filepath.Dir(link), 0755)

	return os.Symlink(target, link)
}

// removeSymlink removes the wgctrl-compat symlink.
func removeSymlink(ifaceName string) {
	link := wgctrlSocketPath(ifaceName)
	os.Remove(link)
}

// StartAWGProcess starts amneziawg-go <ifaceName> and waits for the
// UAPI socket to become available (timeout 10s).
// If the socket already exists the process is considered already running
// and nil is returned (idempotent).
func StartAWGProcess(ifaceName string) error {
	mu.Lock()
	if _, running := procs[ifaceName]; running {
		mu.Unlock()
		return nil // already managed by us
	}
	mu.Unlock()

	sock := socketPath(ifaceName)
	if _, err := os.Stat(sock); err == nil {
		return nil // already running (managed externally)
	}

	cmd := exec.Command("amneziawg-go", ifaceName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("awg: start %s failed: %w", ifaceName, err)
	}

	// Wait for socket with 10s timeout
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			// Create wgctrl-compat symlink
			if slErr := ensureSymlink(ifaceName); slErr != nil {
				// non-fatal: log context preserved in caller
				_ = slErr
			}
			mu.Lock()
			procs[ifaceName] = cmd
			mu.Unlock()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Timeout — kill the process
	cmd.Process.Kill()
	return fmt.Errorf("awg: %s socket did not appear within 10s", ifaceName)
}

// StopAWGProcess sends SIGTERM to the amneziawg-go process for the given
// interface and cleans up the symlink.
func StopAWGProcess(ifaceName string) error {
	mu.Lock()
	cmd, ok := procs[ifaceName]
	if ok {
		delete(procs, ifaceName)
	}
	mu.Unlock()

	removeSymlink(ifaceName)

	if !ok {
		// Not managed by us; try pkill as fallback
		kill := exec.Command("pkill", "-f", "amneziawg-go "+ifaceName)
		kill.Run() // best effort
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		// SIGTERM may not work everywhere; fall back to Kill
		cmd.Process.Kill()
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}

	return nil
}

// StopAllAWGProcesses stops all tracked amneziawg-go processes.
func StopAllAWGProcesses() {
	mu.Lock()
	names := make([]string, 0, len(procs))
	for n := range procs {
		names = append(names, n)
	}
	mu.Unlock()

	for _, n := range names {
		StopAWGProcess(n)
	}
}

// IsAWGProcessRunning returns true if the process is tracked and alive.
func IsAWGProcessRunning(ifaceName string) bool {
	mu.Lock()
	cmd, ok := procs[ifaceName]
	mu.Unlock()
	if !ok {
		return false
	}
	return cmd.Process != nil && cmd.ProcessState == nil
}

// ─────────────────────────────────────────────────────────────
// UAPI communication with amneziawg-go
// ─────────────────────────────────────────────────────────────

// ApplyAWGParams writes AWG obfuscation parameters into the UAPI socket.
// Must be called AFTER the standard wgctrl.ConfigureDevice().
// Returns nil if params are zero (no-op).
func ApplyAWGParams(ifaceName string, params AWGParams) error {
	if params.IsZero() {
		return nil
	}

	sock := socketPath(ifaceName)
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return fmt.Errorf("awg: dial %s: %w", sock, err)
	}
	defer conn.Close()

	cmd := fmt.Sprintf(
		"set=1\njc=%d\njmin=%d\njmax=%d\ns1=%d\ns2=%d\nh1=%d\nh2=%d\nh3=%d\nh4=%d\n\n",
		params.Jc, params.Jmin, params.Jmax,
		params.S1, params.S2,
		params.H1, params.H2, params.H3, params.H4,
	)
	if _, err := fmt.Fprint(conn, cmd); err != nil {
		return fmt.Errorf("awg: write %s: %w", ifaceName, err)
	}

	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	resp := string(buf[:n])
	if !strings.Contains(resp, "errno=0") {
		return fmt.Errorf("awg: %s rejected: %q", ifaceName, resp)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────
// Parameter generation
// ─────────────────────────────────────────────────────────────

// GenerateAWGParams creates random AWG obfuscation parameters.
func GenerateAWGParams() (AWGParams, error) {
	rng := func(lo, hi int) int {
		if hi <= lo {
			return lo
		}
		var b [4]byte
		_, _ = rand.Read(b[:])
		n := int(binary.LittleEndian.Uint32(b[:]))
		if n < 0 {
			n = -n
		}
		return lo + n%(hi-lo+1)
	}

	randU32 := func() uint32 {
		for {
			var b [4]byte
			_, _ = rand.Read(b[:])
			v := binary.LittleEndian.Uint32(b[:])
			if v >= 5 { // H values must be ≥ 5
				return v
			}
		}
	}

	// S1 and S2 with S1+56 ≠ S2 constraint
	s1 := rng(15, 150)
	s2 := rng(15, 150)
	for s1+56 == s2 {
		s2 = rng(15, 150)
	}

	// H1–H4 pairwise unique
	hs := make(map[uint32]bool, 4)
	h := func() uint32 {
		for {
			v := randU32()
			if !hs[v] {
				hs[v] = true
				return v
			}
		}
	}

	jmin := rng(50, 100)
	return AWGParams{
		Jc:   rng(3, 10),
		Jmin: jmin,
		Jmax: rng(jmin, 1000),
		S1:   s1,
		S2:   s2,
		H1:   h(), H2: h(), H3: h(), H4: h(),
	}, nil
}
