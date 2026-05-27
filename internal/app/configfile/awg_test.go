package configfile

import (
	"io"
	"strings"
	"testing"

	"github.com/h44z/wg-portal/internal/domain"
)

func mustReadAll(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	return string(data)
}

func cidr(t *testing.T, s string) domain.Cidr {
	t.Helper()
	c, err := domain.CidrFromString(s)
	if err != nil {
		t.Fatalf("CidrFromString(%q) failed: %v", s, err)
	}
	return c
}

func TestGetInterfaceConfig_WithAWGEnabled(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	iface := &domain.Interface{
		Identifier:  "wg0",
		DisplayName: "AWG Test",
		AWGEnabled:  true,
		AWGJc:       5, AWGJmin: 60, AWGJmax: 500,
		AWGS1: 20, AWGS2: 30, AWGS3: 15, AWGS4: 5,
		AWGH1: 100, AWGH2: 200, AWGH3: 300, AWGH4: 400,
		KeyPair: domain.KeyPair{
			PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
			PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
		},
		ListenPort: 51820,
		Addresses:  []domain.Cidr{cidr(t, "10.0.0.1/24")},
	}

	cfg, err := handler.GetInterfaceConfig(iface, nil)
	if err != nil {
		t.Fatalf("GetInterfaceConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	t.Logf("Generated interface config:\n%s", output)

	for _, key := range []string{"Jc = 5", "Jmin = 60", "Jmax = 500", "H1 = 100", "H4 = 400"} {
		if !strings.Contains(output, key) {
			t.Errorf("Missing AWG directive: %s", key)
		}
	}
}

func TestGetInterfaceConfig_WithoutAWG(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	iface := &domain.Interface{
		Identifier:  "wg0",
		DisplayName: "Vanilla WG",
		AWGEnabled:  false,
		AWGJc:       5, // params present but disabled
		KeyPair: domain.KeyPair{
			PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
			PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
		},
		ListenPort: 51820,
		Addresses:  []domain.Cidr{cidr(t, "10.0.0.1/24")},
	}

	cfg, err := handler.GetInterfaceConfig(iface, nil)
	if err != nil {
		t.Fatalf("GetInterfaceConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	if strings.Contains(output, "Jc = ") {
		t.Error("AWG directives found even though AWGEnabled=false")
	}
}

func TestGetPeerConfig_WithAWGEnabled(t *testing.T) {
	handler, err := newTemplateHandler()
	if err != nil {
		t.Fatalf("newTemplateHandler() failed: %v", err)
	}

	peer := &domain.Peer{
		Identifier:  domain.PeerIdentifier("MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc="),
		DisplayName: "AWG Peer",
		Interface: domain.PeerInterfaceConfig{
			KeyPair: domain.KeyPair{
				PrivateKey: "uEGeOEFfQOl/uZid6VFnBwQz/+p8lFYqMKM2fOTFZls=",
				PublicKey:  "MojDxVk+x0RBZ0VHcI4wCjYh8CTDxXBIJ0E8J5HH1Dc=",
			},
			AWGEnabled: true,
			AWGJc: 7, AWGJmin: 55, AWGJmax: 700,
			AWGS1: 25, AWGS2: 35, AWGS3: 12, AWGS4: 3,
			AWGH1: 101, AWGH2: 202, AWGH3: 303, AWGH4: 404,
		},
		EndpointPublicKey: domain.NewConfigOption("xTU4q8UYv7dWgE5kF2jL9zC1bN0mR3pS6tH8wK4yA5=", true),
		Endpoint:          domain.NewConfigOption("vpn.example.com:51820", true),
		AllowedIPsStr:     domain.NewConfigOption("0.0.0.0/0", true),
	}

	cfg, err := handler.GetPeerConfig(peer, "wgquick")
	if err != nil {
		t.Fatalf("GetPeerConfig() failed: %v", err)
	}

	output := mustReadAll(t, cfg)
	t.Logf("Generated peer config:\n%s", output)

	for _, key := range []string{"Jc = 7", "Jmin = 55", "H1 = 101", "H4 = 404"} {
		if !strings.Contains(output, key) {
			t.Errorf("Missing AWG directive in peer config: %s", key)
		}
	}
}
