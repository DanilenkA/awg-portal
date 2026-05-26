package configfile

import (
	"io"
	"strings"
	"testing"

	"github.com/h44z/wg-portal/internal/domain"
	"github.com/h44z/wg-portal/internal/lowlevel"
)

func mustReadAll(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	return string(data)
}

func mustParseCidr(t *testing.T, s string) domain.Cidr {
	t.Helper()
	c, err := domain.CidrFromString(s)
	if err != nil {
		t.Fatalf("CidrFromString(%q) failed: %v", s, err)
	}
	return c
}

func TestGetInterfaceConfig_WithAWGParams(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	iface := &domain.Interface{
		Identifier:  "wg0",
		DisplayName: "Test Interface",
		KeyPair: domain.KeyPair{
			PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
			PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
		},
		ListenPort: 51820,
		Addresses:  []domain.Cidr{mustParseCidr(t, "10.0.0.1/24")},
		AWGJc:      5,
		AWGJmin:    10,
		AWGJmax:    40,
		AWGS1:      20,
		AWGS2:      30,
		AWGH1:      100,
		AWGH2:      200,
		AWGH3:      300,
		AWGH4:      400,
	}

	cfg, err := handler.GetInterfaceConfig(iface, nil)
	if err != nil {
		t.Fatalf("GetInterfaceConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	t.Logf("Generated interface config:\n%s", output)

	checks := []struct {
		key      string
		expected string
	}{
		{"Jc", "Jc = 5"},
		{"Jmin", "Jmin = 10"},
		{"Jmax", "Jmax = 40"},
		{"S1", "S1 = 20"},
		{"S2", "S2 = 30"},
		{"H1", "H1 = 100"},
		{"H2", "H2 = 200"},
		{"H3", "H3 = 300"},
		{"H4", "H4 = 400"},
	}
	for _, c := range checks {
		if !strings.Contains(output, c.expected) {
			t.Errorf("Missing AWG directive in config: %s (expected: %s)", c.key, c.expected)
		}
	}
}

func TestGetInterfaceConfig_WithoutAWGParams(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	iface := &domain.Interface{
		Identifier:  "wg0",
		DisplayName: "Test Interface",
		KeyPair: domain.KeyPair{
			PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
			PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
		},
		ListenPort: 51820,
		Addresses:  []domain.Cidr{mustParseCidr(t, "10.0.0.1/24")},
		// No AWG params set
	}

	cfg, err := handler.GetInterfaceConfig(iface, nil)
	if err != nil {
		t.Fatalf("GetInterfaceConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	t.Logf("Generated interface config (no AWG):\n%s", output)

	if strings.Contains(output, "Jc = ") {
		t.Error("AWG directive Jc found in config even though no AWG params were set")
	}
}

func TestGetPeerConfig_WithAWGParams(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	peer := &domain.Peer{
		Identifier: domain.PeerIdentifier("MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc="),
		DisplayName: "Test Peer",
		Interface: domain.PeerInterfaceConfig{
			KeyPair: domain.KeyPair{
				PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
				PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
			},
			AWGJc:   7,
			AWGJmin: 12,
			AWGJmax: 45,
			AWGS1:   25,
			AWGS2:   35,
			AWGH1:   101,
			AWGH2:   202,
			AWGH3:   303,
			AWGH4:   404,
		},
		EndpointPublicKey: domain.NewConfigOption("D7L3UfJkLv8k9zC4aN5cR6bQ7wE8tY9uI0oP1a2S3d4=", true),
		Endpoint:          domain.NewConfigOption("vpn.example.com:51820", true),
		AllowedIPsStr:     domain.NewConfigOption("0.0.0.0/0, ::/0", true),
		PresharedKey:      "",
	}

	cfg, err := handler.GetPeerConfig(peer, "wgquick")
	if err != nil {
		t.Fatalf("GetPeerConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	t.Logf("Generated peer config:\n%s", output)

	checks := []string{
		"Jc = 7", "Jmin = 12", "Jmax = 45",
		"S1 = 25", "S2 = 35",
		"H1 = 101", "H2 = 202", "H3 = 303", "H4 = 404",
	}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("Missing AWG directive in peer config: %s", c)
		}
	}
}

func TestAWGParams_GeneratedEnsureValid(t *testing.T) {
	p, err := lowlevel.GenerateAWGParams()
	if err != nil {
		t.Fatalf("GenerateAWGParams() failed: %v", err)
	}

	if p.Jc < 3 || p.Jc > 10 {
		t.Errorf("Jc out of range [3,10]: got %d (should be catched by lowlevel tests)", p.Jc)
	}
}
