package wgcontroller

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/Jipok/wgctrl-go/wgtypes"
	"golang.org/x/sys/unix"

	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
	"github.com/DanilenkA/awg-portal/internal/lowlevel"
)

func TestIsStaleAWGSocketError(t *testing.T) {
	if !isStaleAWGSocketError(fmt.Errorf("wgctrl dial failed: %w", unix.ECONNREFUSED)) {
		t.Fatal("expected wrapped ECONNREFUSED to be treated as stale AWG socket")
	}

	if isStaleAWGSocketError(errors.New("connection refused")) {
		t.Fatal("plain string error must not be treated as stale AWG socket")
	}
}

func TestShouldTryAWG(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		needsAWG bool
		want     bool
	}{
		{name: "always tries for plain WG", mode: config.AWGModeAlways, needsAWG: false, want: true},
		{name: "always tries for AWG", mode: config.AWGModeAlways, needsAWG: true, want: true},
		{name: "auto skips plain WG", mode: config.AWGModeAuto, needsAWG: false, want: false},
		{name: "auto tries AWG", mode: config.AWGModeAuto, needsAWG: true, want: true},
		{name: "never skips plain WG", mode: config.AWGModeNever, needsAWG: false, want: false},
		{name: "never skips AWG", mode: config.AWGModeNever, needsAWG: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldTryAWG(tt.mode, tt.needsAWG); got != tt.want {
				t.Fatalf("shouldTryAWG(%q, %v) = %v, want %v", tt.mode, tt.needsAWG, got, tt.want)
			}
		})
	}
}

func TestUpdateWireGuardInterface_AWGModeNeverSuppressesAWGFields(t *testing.T) {
	keyPair, err := domain.NewFreshKeypair()
	if err != nil {
		t.Fatalf("failed to create keypair: %v", err)
	}

	pi := &domain.PhysicalInterface{
		Identifier: "wg0",
		KeyPair:    keyPair,
		ListenPort: 51820,
		AWGEnabled: true,
	}
	pi.SetAWGParams(lowlevel.AWGParams{
		Jc:   4,
		Jmin: 64,
		Jmax: 128,
		S1:   10,
		S2:   20,
		S3:   30,
		S4:   40,
		H1:   100,
		H2:   200,
		H3:   300,
		H4:   400,
	})

	wg := &capturingWgCtrlRepo{}
	controller := LocalController{
		cfg: &config.Config{Backend: config.Backend{AWGMode: config.AWGModeNever}},
		wg:  wg,
	}

	if err := controller.updateWireGuardInterface(pi); err != nil {
		t.Fatalf("updateWireGuardInterface returned error: %v", err)
	}

	if wg.config.Jc != nil || wg.config.Jmin != nil || wg.config.Jmax != nil ||
		wg.config.S1 != nil || wg.config.S2 != nil || wg.config.S3 != nil || wg.config.S4 != nil ||
		wg.config.H1 != nil || wg.config.H2 != nil || wg.config.H3 != nil || wg.config.H4 != nil {
		t.Fatalf("expected awg_mode=never to suppress AWG fields, got config: %+v", wg.config)
	}
}

type capturingWgCtrlRepo struct {
	config wgtypes.Config
}

func (c *capturingWgCtrlRepo) Close() error { return nil }

func (c *capturingWgCtrlRepo) Devices() ([]*wgtypes.Device, error) { return nil, nil }

func (c *capturingWgCtrlRepo) Device(_ string) (*wgtypes.Device, error) { return nil, nil }

func (c *capturingWgCtrlRepo) ConfigureDevice(_ string, cfg wgtypes.Config) error {
	c.config = cfg
	return nil
}

// fakeWgCtrlWithDevice lets us simulate either a successful kernel
// response, an os.ErrNotExist error, or an arbitrary "weird" error.
type fakeWgCtrlWithDevice struct {
	dev  *wgtypes.Device
	err  error
	hits int
}

func (f *fakeWgCtrlWithDevice) Close() error { return nil }

func (f *fakeWgCtrlWithDevice) Devices() ([]*wgtypes.Device, error) { return nil, nil }

func (f *fakeWgCtrlWithDevice) Device(_ string) (*wgtypes.Device, error) {
	f.hits++
	return f.dev, f.err
}

func (f *fakeWgCtrlWithDevice) ConfigureDevice(_ string, _ wgtypes.Config) error {
	return nil
}

func TestGetDeviceWithAWGFallback_NoError(t *testing.T) {
	// When the kernel client returns the device directly, the fallback
	// must NOT be invoked: the same device is returned unchanged.
	want := &wgtypes.Device{Name: "wg0", ListenPort: 12345}
	fake := &fakeWgCtrlWithDevice{dev: want}
	controller := LocalController{
		cfg: &config.Config{Backend: config.Backend{AWGMode: config.AWGModeAlways}},
		wg:  fake,
	}
	got, err := controller.getDeviceWithAWGFallback("wg0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("expected kernel device to be returned unchanged, got %+v", got)
	}
	if fake.hits != 1 {
		t.Fatalf("kernel client hit count = %d, want 1", fake.hits)
	}
}

func TestGetDeviceWithAWGFallback_NotExistPassthrough(t *testing.T) {
	// When the kernel client returns os.ErrNotExist the fallback must
	// surface that exact error so the caller's "not found" path
	// continues to work.
	fake := &fakeWgCtrlWithDevice{err: os.ErrNotExist}
	controller := LocalController{
		cfg: &config.Config{Backend: config.Backend{AWGMode: config.AWGModeAlways}},
		wg:  fake,
	}
	dev, err := controller.getDeviceWithAWGFallback("wg0")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got: %v", err)
	}
	if dev != nil {
		t.Fatalf("expected nil device, got %+v", dev)
	}
}

func TestGetDeviceWithAWGFallback_OtherErrorAWGNever(t *testing.T) {
	// When the kernel client returns a "weird" error and AWG mode is
	// `never`, the fallback must not engage — the original error is
	// returned as-is.
	fake := &fakeWgCtrlWithDevice{err: errors.New("some kernel netlink error")}
	controller := LocalController{
		cfg: &config.Config{Backend: config.Backend{AWGMode: config.AWGModeNever}},
		wg:  fake,
	}
	_, err := controller.getDeviceWithAWGFallback("wg0")
	if err == nil || err.Error() != "some kernel netlink error" {
		t.Fatalf("expected original error to be returned unchanged, got: %v", err)
	}
}

func TestGetDeviceWithAWGFallback_NoSocketPath(t *testing.T) {
	// When the kernel client returns a "weird" error but the UAPI
	// socket for the interface is missing, the fallback must not
	// engage (no AWG process to talk to) and the original error is
	// returned as-is.
	fake := &fakeWgCtrlWithDevice{err: errors.New("kernel: ENOTSUP")}
	controller := LocalController{
		cfg: &config.Config{Backend: config.Backend{AWGMode: config.AWGModeAlways}},
		wg:  fake,
	}
	// Use a unique iface name that won't have a UAPI socket file.
	_, err := controller.getDeviceWithAWGFallback("definitely-nonexistent-test-iface-1234567890")
	if err == nil || err.Error() != "kernel: ENOTSUP" {
		t.Fatalf("expected original error to be returned unchanged, got: %v", err)
	}
}
