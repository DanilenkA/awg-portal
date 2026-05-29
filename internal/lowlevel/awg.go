package lowlevel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
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
// AWG obfuscation parameters (amneziawg-go 2.x)
// ─────────────────────────────────────────────────────────────

// AWGParams holds AmneziaWG DPI-obfuscation parameters.
// All zero values = vanilla WireGuard (backward compatible).
type AWGParams struct {
	Jc         int    // junk packets before handshake [0–10]
	Jmin, Jmax int    // junk size range in bytes
	S1, S2, S3, S4 int // random prefix bytes per packet type
	H1, H2, H3, H4 uint32 // message type values (single value; ranges via "start-end" in UAPI)
}

// IsZero returns true when all fields are at their zero value.
func (p AWGParams) IsZero() bool {
	return p.Jc == 0 && p.H1 == 0 && p.H2 == 0 && p.H3 == 0 && p.H4 == 0
}

// ─────────────────────────────────────────────────────────────
// Process management
// ─────────────────────────────────────────────────────────────

var (
	mu      sync.Mutex
	procs   = make(map[string]*exec.Cmd) // ifaceName → Cmd
	sockDir = "/var/run/amneziawg"        // amneziawg-go hardcodes this
)

func SocketPath(ifaceName string) string {
	return filepath.Join(sockDir, ifaceName+".sock")
}

// StartAWGProcess starts "amneziawg-go --foreground <ifaceName>" and
// waits for the UAPI socket (timeout 10s).
// If a stale socket exists (file present but no listener), it is cleaned up
// and the process is restarted.
func StartAWGProcess(ifaceName string) error {
	mu.Lock()
	if _, running := procs[ifaceName]; running {
		mu.Unlock()
		return nil // already managed by us
	}
	mu.Unlock()

	sock := SocketPath(ifaceName)
	if _, err := os.Stat(sock); err == nil {
		// Socket exists — verify it's actually listening (stale socket check)
		if conn, dialErr := net.DialTimeout("unix", sock, 500*time.Millisecond); dialErr == nil {
			conn.Close()
			return nil // socket is alive
		}
		// Stale socket — remove it so we can start fresh
		os.Remove(sock)
	}

	// --foreground is REQUIRED: without it amneziawg-go forks to
	// background and the parent PID dies, making StopAWGProcess
	// (SIGTERM) non-functional.
	cmd := exec.Command("amneziawg-go", "--foreground", ifaceName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("awg: start %s failed: %w", ifaceName, err)
	}

	// Wait for socket with 10s timeout
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
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

// StopAWGProcess sends SIGTERM to the amneziawg-go process.
func StopAWGProcess(ifaceName string) error {
	mu.Lock()
	cmd, ok := procs[ifaceName]
	if ok {
		delete(procs, ifaceName)
	}
	mu.Unlock()

	if !ok {
		// Not managed by us; try pkill as fallback
		kill := exec.Command("pkill", "-f", "amneziawg-go "+ifaceName)
		kill.Run() // best effort
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
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
// UAPI direct peer management (bypasses wgctrl-go parse issues)
// ─────────────────────────────────────────────────────────────

// AWGUAPIPeerConfig holds the minimal configuration needed to add or update
// a peer via the AmneziaWG UAPI socket.
type AWGUAPIPeerConfig struct {
	PublicKey    string   // base64-encoded public key
	PresharedKey string   // base64-encoded preshared key (optional, empty = none)
	Endpoint     string   // endpoint host:port (optional, empty = none)
	AllowedIPs   []string // CIDR notation, e.g. ["10.200.0.2/32"]
	Keepalive    int      // persistent keepalive interval in seconds
}

// UAPIKeyToHex converts a base64-encoded WireGuard public key to hex format
// required by the AWG UAPI protocol.
func UAPIKeyToHex(keyB64 string) string {
	raw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

// SetAWGPeer adds or updates a peer on an AWG interface via direct UAPI
// communication with the amneziawg-go process. Uses hex-encoded keys.
func SetAWGPeer(ifaceName string, peer AWGUAPIPeerConfig) error {
	sock := SocketPath(ifaceName)
	conn, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return fmt.Errorf("awg uapi: dial %s: %w", sock, err)
	}
	defer conn.Close()

	pubHex := UAPIKeyToHex(peer.PublicKey)
	if pubHex == "" {
		return fmt.Errorf("awg uapi: invalid public key: %s", peer.PublicKey)
	}

	var buf strings.Builder
	buf.WriteString("set=1\n")
	buf.WriteString(fmt.Sprintf("public_key=%s\n", pubHex))

	// Optional preshared key (hex-encoded)
	if peer.PresharedKey != "" {
		pskHex := UAPIKeyToHex(peer.PresharedKey)
		if pskHex != "" {
			buf.WriteString(fmt.Sprintf("preshared_key=%s\n", pskHex))
		}
	}

	// Optional endpoint
	if peer.Endpoint != "" {
		buf.WriteString(fmt.Sprintf("endpoint=%s\n", peer.Endpoint))
	}

	for _, cidr := range peer.AllowedIPs {
		buf.WriteString(fmt.Sprintf("allowed_ip=%s\n", cidr))
	}
	if peer.Keepalive > 0 {
		buf.WriteString(fmt.Sprintf("persistent_keepalive_interval=%d\n", peer.Keepalive))
	}
	buf.WriteString("\n")

	if _, err := conn.Write([]byte(buf.String())); err != nil {
		return fmt.Errorf("awg uapi: write: %w", err)
	}

	resp := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("awg uapi: read response: %w", err)
	}

	status := strings.TrimSpace(string(resp[:n]))
	if status != "errno=0" && !strings.Contains(status, "errno=0") {
		return fmt.Errorf("awg uapi: %s", status)
	}

	return nil
}

// RemoveAWGPeer removes a peer from an AWG interface via direct UAPI.
func RemoveAWGPeer(ifaceName string, pubKeyB64 string) error {
	sock := SocketPath(ifaceName)
	conn, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return fmt.Errorf("awg uapi: dial %s: %w", sock, err)
	}
	defer conn.Close()

	pubHex := UAPIKeyToHex(pubKeyB64)
	if pubHex == "" {
		return fmt.Errorf("awg uapi: invalid public key: %s", pubKeyB64)
	}

	msg := fmt.Sprintf("set=1\npublic_key=%s\nremove=true\n\n", pubHex)
	if _, err := conn.Write([]byte(msg)); err != nil {
		return fmt.Errorf("awg uapi: write: %w", err)
	}

	resp := make([]byte, 128)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(resp)
	if err != nil {
		return fmt.Errorf("awg uapi: read response: %w", err)
	}

	status := strings.TrimSpace(string(resp[:n]))
	if status != "errno=0" && !strings.Contains(status, "errno=0") {
		return fmt.Errorf("awg uapi: %s", status)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────
// Parameter generation (amneziawg-go 2.x)
// ─────────────────────────────────────────────────────────────

// GenerateAWGParams creates AWG 2.0 obfuscation parameters with proper
// constraints (same as Jipok/wgctrl-go GenerateAmneziaParams):
//   S1-S3: 0-63, S4: 0-15; all padding values unique; total packet sizes unique
//   H1-H4: pairwise unique uint32 values (≥ 5)
//   Jc: 3-6, Jmin: 64-113, Jmax: Jmin+50..Jmin+149
func GenerateAWGParams() (AWGParams, error) {
	rng := func(lo, hi int) int {
		if hi <= lo {
			return lo
		}
		var b [8]byte
		_, _ = rand.Read(b[:])
		return lo + int(binary.LittleEndian.Uint64(b[:])%uint64(hi-lo+1))
	}

	randU32 := func() uint32 {
		for {
			var b [4]byte
			_, _ = rand.Read(b[:])
			v := binary.LittleEndian.Uint32(b[:])
			if v >= 5 {
				return v
			}
		}
	}

	// S1-S4: unique values + unique total packet sizes
	// WG control packet sizes: init=148, resp=92, cookie=64
	var s1, s2, s3, s4 int
	for {
		s1 = rng(15, 63)
		s2 = rng(15, 63)
		s3 = rng(10, 63)
		s4 = rng(1, 15)

		if s1 == s2 || s1 == s3 || s1 == s4 || s2 == s3 || s2 == s4 || s3 == s4 {
			continue // all padding values must be unique
		}
		if s1+148 == s2+92 || s3+64 == s1+148 || s3+64 == s2+92 {
			continue // total packet sizes must be unique
		}
		break
	}

	// H1-H4 pairwise unique, ≥ 5
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

	jmin := rng(64, 113)
	return AWGParams{
		Jc:   rng(3, 6),
		Jmin: jmin,
		Jmax: rng(jmin+50, jmin+149),
		S1:   s1, S2: s2, S3: s3, S4: s4,
		H1: h(), H2: h(), H3: h(), H4: h(),
	}, nil
}
