package lowlevel

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// AWGParams holds AmneziaWG obfuscation parameters.
type AWGParams struct {
	Jc, Jmin, Jmax int
	S1, S2         int
	H1, H2, H3, H4 uint32
}

// IsZero returns true if no AWG obfuscation parameters are set.
func (p AWGParams) IsZero() bool {
	return p.Jc == 0 && p.H1 == 0 && p.H2 == 0 && p.H3 == 0 && p.H4 == 0
}

// ApplyAWGParams writes AWG obfuscation parameters directly into the UAPI socket.
// IMPORTANT: must be called AFTER wgctrl.ConfigureDevice().
// The \n in the format string are real newline characters, not literal backslash-n.
func ApplyAWGParams(ifaceName string, params AWGParams) error {
	if params.IsZero() {
		return nil
	}

	socketPath := fmt.Sprintf("/var/run/wireguard/%s.sock", ifaceName)
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("awg: UAPI socket open failed (%s): %w", socketPath, err)
	}
	defer conn.Close()

	cmd := fmt.Sprintf(
		"set=1\njc=%d\njmin=%d\njmax=%d\ns1=%d\ns2=%d\nh1=%d\nh2=%d\nh3=%d\nh4=%d\n\n",
		params.Jc, params.Jmin, params.Jmax,
		params.S1, params.S2,
		params.H1, params.H2, params.H3, params.H4,
	)
	if _, err := fmt.Fprint(conn, cmd); err != nil {
		return fmt.Errorf("awg: UAPI write failed: %w", err)
	}

	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	resp := string(buf[:n])
	if !strings.Contains(resp, "errno=0") {
		return fmt.Errorf("awg: UAPI rejected params for %s: %q", ifaceName, resp)
	}

	return nil
}

// GenerateAWGParams generates random AmneziaWG obfuscation parameters.
// Jc, Jmin, Jmax control the junk packet insertion behaviour.
// S1, S2 control the size of additional data packets.
// H1–H4 are 32-bit header obfuscation magic values.
func GenerateAWGParams() (AWGParams, error) {
	randU32 := func() uint32 {
		var b [4]byte
		_, _ = rand.Read(b[:])
		return binary.LittleEndian.Uint32(b[:])
	}
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

	jmin := rng(10, 50)
	return AWGParams{
		Jc:   rng(3, 10),
		Jmin: jmin,
		Jmax: rng(jmin, jmin+50),
		S1:   rng(10, 80),
		S2:   rng(10, 80),
		H1:   randU32(),
		H2:   randU32(),
		H3:   randU32(),
		H4:   randU32(),
	}, nil
}
