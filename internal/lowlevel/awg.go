package lowlevel

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jipok/wgctrl-go/wgtypes"
)

// ─────────────────────────────────────────────────────────────
// AWG obfuscation parameters (amneziawg-go 2.x)
// ─────────────────────────────────────────────────────────────

// AWGParams holds AmneziaWG DPI-obfuscation parameters.
// All zero values = vanilla WireGuard (backward compatible).
type AWGParams struct {
	Jc             int    // junk packets before handshake [0–10]
	Jmin, Jmax     int    // junk size range in bytes
	S1, S2, S3, S4 int    // random prefix bytes per packet type
	H1, H2, H3, H4 uint32 // message type values (single value; ranges via "start-end" in UAPI)
}

// IsZero returns true when all obfuscation parameters are at their
// zero value. This is the canonical "the operator has not configured
// any AWG params" predicate — used to gate AWG parameter emission into
// the kernel UAPI / amneziawg-go TUN so a half-configured AWG profile
// never leaks downstream.
//
// We check every AWG field (Jc, Jmin, Jmax, S1..S4, H1..H4) so that
// the result is symmetric with HasAnyAWGParams on the interface side.
func (p AWGParams) IsZero() bool {
	return p.Jc == 0 &&
		p.Jmin == 0 &&
		p.Jmax == 0 &&
		p.S1 == 0 &&
		p.S2 == 0 &&
		p.S3 == 0 &&
		p.S4 == 0 &&
		p.H1 == 0 &&
		p.H2 == 0 &&
		p.H3 == 0 &&
		p.H4 == 0
}

// Validate returns an error if the AWG parameters are out of the
// amneziawg-go supported ranges. Values outside the supported range
// would either be rejected by amneziawg-go or produce a non-working
// obfuscation channel.
func (p AWGParams) Validate() error {
	// Jc (junk packet count before handshake): 1..128 (we allow a wider
	// range than amneziawg-go defaults 1..10 to leave some headroom).
	if p.Jc < 0 || p.Jc > 128 {
		return fmt.Errorf("awg: Jc out of range [0..128]: %d", p.Jc)
	}
	// Jmin/Jmax: 0..1280. Jmax must be > Jmin when Jc > 0.
	if p.Jmin < 0 || p.Jmin > 1280 {
		return fmt.Errorf("awg: Jmin out of range [0..1280]: %d", p.Jmin)
	}
	if p.Jmax < 0 || p.Jmax > 1280 {
		return fmt.Errorf("awg: Jmax out of range [0..1280]: %d", p.Jmax)
	}
	if p.Jc > 0 && p.Jmax <= p.Jmin {
		return fmt.Errorf("awg: Jmax (%d) must be greater than Jmin (%d) when Jc > 0", p.Jmax, p.Jmin)
	}
	// S1..S4: 0..1280
	if p.S1 < 0 || p.S1 > 1280 ||
		p.S2 < 0 || p.S2 > 1280 ||
		p.S3 < 0 || p.S3 > 1280 ||
		p.S4 < 0 || p.S4 > 1280 {
		return fmt.Errorf("awg: S1..S4 out of range [0..1280]: S1=%d S2=%d S3=%d S4=%d",
			p.S1, p.S2, p.S3, p.S4)
	}
	// H1..H4: 5..2^32-1, pairwise unique
	hs := []uint32{p.H1, p.H2, p.H3, p.H4}
	seen := make(map[uint32]struct{}, 4)
	for _, h := range hs {
		if h < 5 {
			return fmt.Errorf("awg: H value must be >= 5 (got %d)", h)
		}
		if _, dup := seen[h]; dup {
			return fmt.Errorf("awg: H values must be pairwise unique (duplicate %d)", h)
		}
		seen[h] = struct{}{}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Process management
// ─────────────────────────────────────────────────────────────

var (
	// procsMu serialises ALL access to the procs map and the underlying
	// amneziawg-go processes. We hold it across the entire lifecycle of
	// Start/Stop/IsRunning so that two concurrent callers cannot both
	// observe "not running" and start their own process. A sync.Map with
	// LoadOrStore is not sufficient: between the Load and the Start we
	// could race a second caller and end up with two processes bound to
	// the same UAPI socket.
	procsMu sync.Mutex
	procs   = make(map[string]*awgProcEntry) // ifaceName → entry
	sockDir = "/var/run/amneziawg"           // amneziawg-go hardcodes this

	// awgProcessStartTimeout bounds how long we wait for the UAPI socket
	// (and a responsive UAPI listener) to become available.
	awgProcessStartTimeout = 10 * time.Second
	// awgUAPIDialTimeout is the per-attempt dial/read timeout when checking
	// the UAPI socket for liveness.
	awgUAPIDialTimeout = 500 * time.Millisecond
)

// awgProcEntry wraps an *exec.Cmd together with state owned by the
// watchdog goroutine. Nobody outside the watchdog reads cmd.ProcessState:
// callers observe process death via done and, when needed, waitErr under
// procsMu. Keeping the entry in procs until Wait() returns also makes a
// concurrent Stop/Start sequence atomic for a given interface: Start sees
// the in-flight process and cannot spawn a second amneziawg-go instance.
type awgProcEntry struct {
	cmd      *exec.Cmd
	done     chan struct{} // closed by the watchdog after cmd.Wait() returns
	waitErr  error         // set by the watchdog under procsMu before done is closed
	stopping bool          // set by StopAWGProcess under procsMu
}

func SocketPath(ifaceName string) string {
	return filepath.Join(sockDir, ifaceName+".sock")
}

// IsAWGAvailable reports whether the "amneziawg-go" binary is reachable
// through the current PATH. We use exec.LookPath so the result is
// authoritative even if the binary lives outside well-known locations
// (e.g. /usr/local/bin on a custom build). Callers should treat the
// false return as "AWG obfuscation cannot be enabled on this host" and
// surface a human-readable error to the operator.
func IsAWGAvailable() bool {
	_, err := exec.LookPath("amneziawg-go")
	return err == nil
}

// StartAWGProcess starts "amneziawg-go --foreground <ifaceName>" and
// waits for the UAPI socket to become live (timeout configurable via
// awgProcessStartTimeout). If a stale socket exists (file present but no
// listener), it is cleaned up and the process is restarted.
//
// Concurrency: process registration is protected by procsMu. Once a process
// has been started, its entry is registered before we wait for UAPI readiness,
// so concurrent callers for the same ifaceName either observe that startup or
// wait for an in-flight stop before trying again.
//
// Lifecycle: the function spawns a single watchdog goroutine per
// amneziawg-go process. That goroutine is the ONLY caller of
// cmd.Wait() — it owns ProcessState exclusively. Once Wait returns it records
// the wait error, removes the iface from the procs map (under procsMu), and
// closes the entry's done channel so any waiter can return.
func StartAWGProcess(ifaceName string) error {
	procsMu.Lock()
	if entry, running := procs[ifaceName]; running {
		if entry != nil && entry.stopping {
			done := entry.done
			procsMu.Unlock()
			<-done
			return StartAWGProcess(ifaceName)
		}
		procsMu.Unlock()
		return nil // already managed by us
	}

	sock := SocketPath(ifaceName)
	if _, err := os.Stat(sock); err == nil {
		// Socket exists — verify it's actually listening (stale socket
		// check). If yes, mark this iface as managed by an external
		// process and bail out — we did not start it, but we should
		// not spawn a competing process.
		if conn, dialErr := net.DialTimeout("unix", sock, awgUAPIDialTimeout); dialErr == nil {
			conn.Close()
			slog.Debug("awg: socket alive on entry, not starting a new process",
				"iface", ifaceName)
			procsMu.Unlock()
			return nil
		}
		// Stale socket — remove it so we can start fresh
		if rmErr := os.Remove(sock); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			procsMu.Unlock()
			return fmt.Errorf("awg: remove stale socket %s: %w", sock, rmErr)
		}
	}

	// --foreground is REQUIRED: without it amneziawg-go forks to
	// background and the parent PID dies, making StopAWGProcess
	// (SIGTERM) non-functional.
	cmd := exec.Command("amneziawg-go", "--foreground", ifaceName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		procsMu.Unlock()
		return fmt.Errorf("awg: start %s failed: %w", ifaceName, err)
	}

	entry := &awgProcEntry{cmd: cmd, done: make(chan struct{})}
	procs[ifaceName] = entry

	// Spawn the watchdog before waiting on the UAPI socket, so early
	// process death is observed via entry.done without reading ProcessState.
	awgStartWatchdog(ifaceName, entry)
	procsMu.Unlock()

	// Wait for the socket to appear AND respond to a UAPI "get=1" probe.
	deadline := time.Now().Add(awgProcessStartTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-entry.done:
			procsMu.Lock()
			waitErr := entry.waitErr
			procsMu.Unlock()
			if waitErr != nil {
				return fmt.Errorf("awg: %s exited during startup: %w", ifaceName, waitErr)
			}
			return fmt.Errorf("awg: %s exited during startup", ifaceName)
		default:
		}
		if uapiReady(sock) {
			slog.Info("awg: process started and UAPI listener is live",
				"iface", ifaceName, "pid", cmd.Process.Pid)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Timeout — stop the tracked process and let the watchdog reap it.
	_ = StopAWGProcess(ifaceName)
	return fmt.Errorf("awg: %s socket did not appear within %s", ifaceName, awgProcessStartTimeout)
}

// awgStartWatchdog spawns the per-process goroutine that calls
// cmd.Wait() exactly once, removes the entry from procs, and closes
// the entry's done channel. This is the canonical "is this process
// alive?" authority: procs[ifaceName] is non-iff alive exactly when
// the entry is present.
//
// The watchdog is the SOLE owner of cmd.Wait() and the SOLE writer of
// cmd.ProcessState. It must therefore not call into anything that
// itself takes procsMu — except the brief Lock/Unlock that protects
// the map mutation and the channel close.
func awgStartWatchdog(ifaceName string, entry *awgProcEntry) {
	cmd := entry.cmd
	go func() {
		// cmd.Wait() will populate cmd.ProcessState exactly once,
		// either when the process exits on its own or when it is
		// killed via StopAWGProcess. We own this call site — no one
		// else is allowed to Wait on this cmd.
		waitErr := cmd.Wait()

		// Once Wait returns, the process is fully reaped. We record
		// waitErr, remove the entry if it is still ours, and close done
		// under procsMu. Readers never touch cmd.ProcessState.
		procsMu.Lock()
		entry.waitErr = waitErr
		if tracked, ok := procs[ifaceName]; ok && tracked == entry {
			delete(procs, ifaceName)
			slog.Info("awg: process exited, removed from procs",
				"iface", ifaceName, "pid", cmd.Process.Pid, "error", waitErr)
		}
		close(entry.done)
		procsMu.Unlock()
	}()
}

// uapiReady returns true if the UAPI socket at sock is connectable AND
// responds to a "get=1" probe with a valid reply. This guards against
// races where amneziawg-go creates the socket file but has not finished
// initialising the listener.
func uapiReady(sock string) bool {
	if _, err := os.Stat(sock); err != nil {
		return false
	}
	conn, err := net.DialTimeout("unix", sock, awgUAPIDialTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(awgUAPIDialTimeout))
	if _, werr := conn.Write([]byte("get=1\n\n")); werr != nil {
		return false
	}
	// We do not need to fully parse the UAPI dump, just confirm the
	// process is producing a reply.
	buf := make([]byte, 16)
	if _, rerr := conn.Read(buf); rerr != nil {
		return false
	}
	return true
}

// StopAWGProcess sends SIGTERM (then SIGKILL on timeout) to the
// amneziawg-go process tracked for ifaceName, then waits for the
// watchdog to finish reaping it.
//
// The watchdog is the sole owner of cmd.Wait() and of cmd.ProcessState.
// StopAWGProcess therefore does NOT call Wait and does NOT read
// ProcessState — it just marks the entry as stopping, signals the OS
// process and waits on the entry's done channel. The entry remains in
// procs until the watchdog reaps the process, preventing a concurrent
// StartAWGProcess from spawning a second process for the same interface.
//
// If the process is not tracked (e.g. an externally-started one), a
// pkill is attempted as a best-effort cleanup.
func StopAWGProcess(ifaceName string) error {
	procsMu.Lock()
	entry, ok := procs[ifaceName]
	if ok {
		entry.stopping = true
	}
	procsMu.Unlock()

	if !ok {
		// Not managed by us; try pkill as fallback
		kill := exec.Command("pkill", "-f", "amneziawg-go "+ifaceName)
		_ = kill.Run() // best effort
		return nil
	}

	if entry == nil || entry.cmd == nil || entry.cmd.Process == nil {
		return nil
	}

	// Send graceful shutdown signal first. amneziawg-go traps SIGTERM
	// and exits cleanly (dumps its UAPI state, removes the TUN device).
	if err := entry.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process is gone or signal failed — fall through to kill.
		slog.Debug("awg: SIGTERM failed, escalating to SIGKILL", "iface", ifaceName, "err", err)
		_ = entry.cmd.Process.Kill()
	}

	// Wait for the watchdog to reap the process via the done channel.
	// We don't poll ProcessState (the watchdog owns it) and we don't
	// call cmd.Wait() (the watchdog owns that too). The close(entry.done)
	// happens under procsMu in the watchdog, so a successful receive
	// here is a happens-after edge with the watchdog's reaping.
	giveUp := time.After(5 * time.Second)
	select {
	case <-entry.done:
		return nil
	case <-giveUp:
		slog.Warn("awg: process did not exit on SIGTERM within 5s; sending SIGKILL",
			"iface", ifaceName, "pid", entry.cmd.Process.Pid)
		_ = entry.cmd.Process.Kill()
		// Still wait for the watchdog to finish reaping so the
		// caller observes a clean post-condition.
		<-entry.done
		return nil
	}
}

// StopAllAWGProcesses stops all tracked amneziawg-go processes.
func StopAllAWGProcesses() {
	procsMu.Lock()
	names := make([]string, 0, len(procs))
	for n := range procs {
		names = append(names, n)
	}
	procsMu.Unlock()

	for _, n := range names {
		_ = StopAWGProcess(n)
	}
}

// IsAWGProcessRunning returns true if the process is tracked and not currently
// stopping. It does not inspect cmd.ProcessState; the watchdog removes the map
// entry and closes done after Wait() returns.
func IsAWGProcessRunning(ifaceName string) bool {
	procsMu.Lock()
	defer procsMu.Unlock()

	entry, ok := procs[ifaceName]
	if !ok || entry == nil || entry.cmd == nil || entry.cmd.Process == nil {
		return false
	}
	return !entry.stopping
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
//
//	S1-S3: 0-63, S4: 0-15; all padding values unique; total packet sizes unique
//	H1-H4: pairwise unique uint32 values (≥ 5)
//	Jc: 3-6, Jmin: 64-113, Jmax: Jmin+50..Jmin+149
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

// ─────────────────────────────────────────────────────────────
// Direct UAPI device dump (fallback for AWG TUN interfaces)
// ─────────────────────────────────────────────────────────────

// AWGDeviceFromUAPI reads a device dump (peers, keys, listen_port, traffic
// counters) directly from the AmneziaWG UAPI socket for ifaceName. This is
// the fallback path used when the kernel netlink client in wgctrl-go rejects
// an AWG TUN device with a non-`os.ErrNotExist` error (the wireguard
// genetlink family has no record of the AWG TUN, so the wrapper
// short-circuits and never falls through to the userspace client).
//
// Returns os.ErrNotExist when the UAPI socket file is missing — callers
// can then treat the device as not-managed-by-AWG and proceed normally.
//
// Protocol (per https://www.wireguard.com/xplatform/#cross-platform-userspace-implementation):
//   - send "get=1\n\n"
//   - read line-by-line until blank line
//   - parse key=value pairs
//
// We implement the UAPI parser locally (rather than reusing the unexported
// wguser package internals) so the dependency stays on public types only.
func AWGDeviceFromUAPI(ifaceName string) (*wgtypes.Device, error) {
	sock := SocketPath(ifaceName)
	if _, err := os.Stat(sock); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("awg uapi: stat %s: %w", sock, err)
	}

	conn, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("awg uapi: dial %s: %w", sock, err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "get=1\n\n"); err != nil {
		return nil, fmt.Errorf("awg uapi: write get=1: %w", err)
	}

	// Cap the read so a misbehaving server can't keep us reading forever.
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, fmt.Errorf("awg uapi: set read deadline: %w", err)
	}

	dev, err := parseAWGUAPIDevice(conn)
	if err != nil {
		return nil, err
	}
	dev.Name = ifaceName
	dev.Type = wgtypes.Userspace
	// The userspace protocol doesn't expose the public key directly; it
	// is derived from the private key (mirrors wguser.parseDevice).
	dev.PublicKey = dev.PrivateKey.PublicKey()
	return dev, nil
}

// parseAWGUAPIDevice parses a WireGuard userspace UAPI dump from r.
// On success it returns a populated *wgtypes.Device with Type set to
// the kernel default; the caller is expected to overwrite Type to
// wgtypes.Userspace and set Name after the fact.
func parseAWGUAPIDevice(r io.Reader) (*wgtypes.Device, error) {
	var (
		dev    wgtypes.Device
		cur    *wgtypes.Peer
		hsSec  int64
		hsNano int64
	)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			break // empty line terminates dump
		}
		eq := bytesIndexByte(line, '=')
		if eq < 0 {
			continue // not key=value, skip
		}
		key := string(line[:eq])
		val := string(line[eq+1:])

		switch key {
		case "errno":
			if errnoVal, perr := strconv.Atoi(val); perr == nil && errnoVal != 0 {
				return nil, fmt.Errorf("awg uapi: errno=%d", errnoVal)
			}
		case "public_key":
			// New peer entry. Public_key on the device dump line signals
			// the start of a peer block.
			dev.Peers = append(dev.Peers, wgtypes.Peer{})
			cur = &dev.Peers[len(dev.Peers)-1]
			cur.PublicKey = parseUAPIHexKey(val)
		default:
			if cur != nil {
				parseAWGUAPIPeerField(cur, &hsSec, &hsNano, key, val)
			} else {
				parseAWGUIDeviceField(&dev, key, val)
			}
		}
	}
	if serr := scanner.Err(); serr != nil && !errors.Is(serr, io.EOF) {
		return nil, fmt.Errorf("awg uapi: scan: %w", serr)
	}
	return &dev, nil
}

// bytesIndexByte is a local equivalent of bytes.IndexByte that avoids
// pulling in the bytes package for a single call site.
func bytesIndexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// parseUAPIHexKey decodes a hex-encoded WireGuard key (32 bytes when
// correctly formatted) and returns the wgtypes.Key.
func parseUAPIHexKey(s string) wgtypes.Key {
	raw, derr := hex.DecodeString(s)
	if derr != nil || len(raw) != wgtypes.KeyLen {
		return wgtypes.Key{}
	}
	k, kerr := wgtypes.NewKey(raw)
	if kerr != nil {
		return wgtypes.Key{}
	}
	return k
}

// parseAWGUIDeviceField populates device-level fields from a UAPI
// key/value pair. Unknown keys are silently ignored (forward compat).
func parseAWGUIDeviceField(dev *wgtypes.Device, key, val string) {
	switch key {
	case "private_key":
		dev.PrivateKey = parseUAPIHexKey(val)
	case "listen_port":
		if n, err := strconv.Atoi(val); err == nil {
			dev.ListenPort = n
		}
	case "fwmark":
		if n, err := strconv.Atoi(val); err == nil {
			dev.FirewallMark = n
		}
	}
}

// parseAWGUAPIPeerField populates a peer field from a UAPI key/value
// pair. The hsSec/hsNano accumulators carry the split-handshake
// timestamp until both halves are seen.
func parseAWGUAPIPeerField(p *wgtypes.Peer, hsSec, hsNano *int64, key, val string) {
	switch key {
	case "preshared_key":
		p.PresharedKey = parseUAPIHexKey(val)
	case "endpoint":
		if addr, err := net.ResolveUDPAddr("udp", val); err == nil {
			p.Endpoint = addr
		}
	case "last_handshake_time_sec":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			*hsSec = n
		}
	case "last_handshake_time_nsec":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			*hsNano = n
		}
		if *hsSec > 0 || *hsNano > 0 {
			p.LastHandshakeTime = time.Unix(*hsSec, *hsNano)
		}
	case "tx_bytes":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			p.TransmitBytes = n
		}
	case "rx_bytes":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			p.ReceiveBytes = n
		}
	case "persistent_keepalive_interval":
		if n, err := strconv.Atoi(val); err == nil {
			p.PersistentKeepaliveInterval = time.Duration(n) * time.Second
		}
	case "protocol_version":
		if n, err := strconv.Atoi(val); err == nil {
			p.ProtocolVersion = n
		}
	case "allowed_ip":
		if _, ipnet, err := net.ParseCIDR(val); err == nil {
			p.AllowedIPs = append(p.AllowedIPs, *ipnet)
		}
	}
}
