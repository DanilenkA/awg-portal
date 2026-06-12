package wgcontroller

import (
	"errors"
	"fmt"
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
