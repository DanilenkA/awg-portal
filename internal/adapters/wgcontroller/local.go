package wgcontroller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/Jipok/wgctrl-go"
	"github.com/Jipok/wgctrl-go/wgtypes"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DanilenkA/awg-portal/internal"
	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
	"github.com/DanilenkA/awg-portal/internal/lowlevel"
)

// region dependencies

// WgCtrlRepo is used to control local WireGuard devices via the wgctrl-go library.
type WgCtrlRepo interface {
	io.Closer
	Devices() ([]*wgtypes.Device, error)
	Device(name string) (*wgtypes.Device, error)
	ConfigureDevice(name string, cfg wgtypes.Config) error
}

// A NetlinkClient is a type which can control a netlink device.
type NetlinkClient interface {
	LinkAdd(link netlink.Link) error
	LinkDel(link netlink.Link) error
	LinkByName(name string) (netlink.Link, error)
	LinkSetUp(link netlink.Link) error
	LinkSetDown(link netlink.Link) error
	LinkSetMTU(link netlink.Link, mtu int) error
	AddrReplace(link netlink.Link, addr *netlink.Addr) error
	AddrAdd(link netlink.Link, addr *netlink.Addr) error
	AddrList(link netlink.Link) ([]netlink.Addr, error)
	AddrDel(link netlink.Link, addr *netlink.Addr) error
	RouteAdd(route *netlink.Route) error
	RouteDel(route *netlink.Route) error
	RouteReplace(route *netlink.Route) error
	RouteList(link netlink.Link, family int) ([]netlink.Route, error)
	RouteListFiltered(family int, filter *netlink.Route, filterMask uint64) ([]netlink.Route, error)
	RuleAdd(rule *netlink.Rule) error
	RuleDel(rule *netlink.Rule) error
	RuleList(family int) ([]netlink.Rule, error)
}

// endregion dependencies

type LocalController struct {
	cfg *config.Config

	wg WgCtrlRepo
	nl NetlinkClient

	shellCmd              string
	resolvConfIfacePrefix string
}

// NewLocalController creates a new local controller instance.
// This repository is used to interact with the WireGuard kernel or userspace module.
func NewLocalController(cfg *config.Config) (*LocalController, error) {
	wg, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	nl := &lowlevel.NetlinkManager{}

	repo := &LocalController{
		cfg: cfg,

		wg: wg,
		nl: nl,

		shellCmd:              "bash",                            // we only support bash at the moment
		resolvConfIfacePrefix: cfg.Backend.LocalResolvconfPrefix, // WireGuard interfaces have a tun. prefix in resolvconf
	}

	return repo, nil
}

func (c LocalController) GetId() domain.InterfaceBackend {
	return config.LocalBackendName
}

// region wireguard-related

func (c LocalController) GetInterfaces(_ context.Context) ([]domain.PhysicalInterface, error) {
	devices, err := c.wg.Devices()
	if err != nil {
		return nil, fmt.Errorf("device list error: %w", err)
	}

	interfaces := make([]domain.PhysicalInterface, 0, len(devices))
	for _, device := range devices {
		interfaceModel, err := c.convertWireGuardInterface(device)
		if err != nil {
			return nil, fmt.Errorf("interface convert failed for %s: %w", device.Name, err)
		}
		interfaces = append(interfaces, interfaceModel)
	}

	return interfaces, nil
}

func (c LocalController) GetInterface(_ context.Context, id domain.InterfaceIdentifier) (
	*domain.PhysicalInterface,
	error,
) {
	return c.getInterface(id)
}

func (c LocalController) convertWireGuardInterface(device *wgtypes.Device) (domain.PhysicalInterface, error) {
	// read data from wgctrl interface

	iface := domain.PhysicalInterface{
		Identifier: domain.InterfaceIdentifier(device.Name),
		KeyPair: domain.KeyPair{
			PrivateKey: device.PrivateKey.String(),
			PublicKey:  device.PublicKey.String(),
		},
		ListenPort:    device.ListenPort,
		Addresses:     nil,
		Mtu:           0,
		FirewallMark:  uint32(device.FirewallMark),
		DeviceUp:      false,
		ImportSource:  domain.ControllerTypeLocal,
		DeviceType:    device.Type.String(),
		BytesUpload:   0,
		BytesDownload: 0,
	}

	// read data from netlink interface

	lowLevelInterface, err := c.nl.LinkByName(device.Name)
	if err != nil {
		return domain.PhysicalInterface{}, fmt.Errorf("netlink error for %s: %w", device.Name, err)
	}
	ipAddresses, err := c.nl.AddrList(lowLevelInterface)
	if err != nil {
		return domain.PhysicalInterface{}, fmt.Errorf("ip read error for %s: %w", device.Name, err)
	}

	for _, addr := range ipAddresses {
		iface.Addresses = append(iface.Addresses, domain.CidrFromNetlinkAddr(addr))
	}
	iface.Mtu = lowLevelInterface.Attrs().MTU
	iface.DeviceUp = lowLevelInterface.Attrs().OperState == netlink.OperUnknown // wg only supports unknown
	if stats := lowLevelInterface.Attrs().Statistics; stats != nil {
		iface.BytesUpload = stats.TxBytes
		iface.BytesDownload = stats.RxBytes
	}

	return iface, nil
}

func (c LocalController) GetPeers(_ context.Context, deviceId domain.InterfaceIdentifier) (
	[]domain.PhysicalPeer,
	error,
) {
	device, err := c.getDeviceWithAWGFallback(string(deviceId))
	if err != nil {
		return nil, fmt.Errorf("device error: %w", err)
	}

	peers := make([]domain.PhysicalPeer, 0, len(device.Peers))
	for _, peer := range device.Peers {
		peerModel, err := c.convertWireGuardPeer(&peer)
		if err != nil {
			return nil, fmt.Errorf("peer convert failed for %v: %w", peer.PublicKey, err)
		}
		peers = append(peers, peerModel)
	}

	return peers, nil
}

func (c LocalController) convertWireGuardPeer(peer *wgtypes.Peer) (domain.PhysicalPeer, error) {
	peerModel := domain.PhysicalPeer{
		Identifier: domain.PeerIdentifier(peer.PublicKey.String()),
		Endpoint:   "",
		AllowedIPs: nil,
		KeyPair: domain.KeyPair{
			PublicKey: peer.PublicKey.String(),
		},
		PresharedKey:        "",
		PersistentKeepalive: int(peer.PersistentKeepaliveInterval.Seconds()),
		LastHandshake:       peer.LastHandshakeTime,
		ProtocolVersion:     peer.ProtocolVersion,
		BytesUpload:         uint64(peer.ReceiveBytes),
		BytesDownload:       uint64(peer.TransmitBytes),
		ImportSource:        domain.ControllerTypeLocal,
	}

	// Set local extras - local peers are never disabled in the kernel
	peerModel.SetExtras(domain.LocalPeerExtras{
		Disabled: false,
	})

	for _, addr := range peer.AllowedIPs {
		peerModel.AllowedIPs = append(peerModel.AllowedIPs, domain.CidrFromIpNet(addr))
	}
	if peer.Endpoint != nil {
		peerModel.Endpoint = peer.Endpoint.String()
	}
	if peer.PresharedKey != (wgtypes.Key{}) {
		peerModel.PresharedKey = domain.PreSharedKey(peer.PresharedKey.String())
	}

	return peerModel, nil
}

func (c LocalController) SaveInterface(
	_ context.Context,
	id domain.InterfaceIdentifier,
	updateFunc func(pi *domain.PhysicalInterface) (*domain.PhysicalInterface, error),
) error {
	// Pre-apply updateFunc to a zero-value PhysicalInterface to determine
	// AWG requirements BEFORE creating the low-level interface.
	// This avoids creating a TUN (via amneziawg-go) for plain WG interfaces.
	needsAWG := false
	if updateFunc != nil {
		probePI := &domain.PhysicalInterface{Identifier: id}
		if probedPI, probeErr := updateFunc(probePI); probeErr == nil {
			needsAWG = probedPI.EmitAWG()
		}
	}

	physicalInterface, err := c.getOrCreateInterface(id, needsAWG)
	if err != nil {
		return err
	}

	if updateFunc != nil {
		physicalInterface, err = updateFunc(physicalInterface)
		if err != nil {
			return err
		}
	}

	if err := c.updateLowLevelInterface(physicalInterface); err != nil {
		return err
	}
	if err := c.updateWireGuardInterface(physicalInterface); err != nil {
		return err
	}

	// HACK: amneziawg-go TUN interfaces don't get automatic connected routes.
	// Add them here as a best-effort first attempt; SetRoutes will re-add them
	// after the route manager clears stale entries.
	if err := c.ensureAWGConnectedRoute(physicalInterface); err != nil {
		return err
	}

	return nil
}

func (c LocalController) getOrCreateInterface(id domain.InterfaceIdentifier, needsAWG bool) (*domain.PhysicalInterface, error) {
	device, err := c.getInterface(id)
	if err == nil {
		return device, nil // interface exists
	}

	// If the error is a stale AWG socket (connection refused), recover by
	// stopping the dead process, removing the socket, and recreating.
	// This handles the case where amneziawg-go died but left the socket file behind.
	if errors.Is(err, os.ErrNotExist) {
		// interface doesn't exist at all — create it
		if err := c.createLowLevelInterface(id, needsAWG); err != nil {
			return nil, err
		}
		return c.getInterface(id)
	}

	// Check for stale AWG socket (wgctrl-go dial unix socket failure)
	if isStaleAWGSocketError(err) && c.cfg.Backend.GetAWGMode() != config.AWGModeNever {
		slog.Warn("stale AWG socket detected, restarting amneziawg-go", "iface", id)
		// Kill any orphaned amneziawg-go process for this interface
		_ = lowlevel.StopAWGProcess(string(id))
		// Remove stale socket file
		sockPath := lowlevel.SocketPath(string(id))
		if removeErr := os.Remove(sockPath); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("failed to remove stale AWG socket", "path", sockPath, "err", removeErr)
		}
		// Recreate interface via lowlevel (starts AWG process)
		if createErr := c.createLowLevelInterface(id, needsAWG); createErr != nil {
			return nil, fmt.Errorf("failed to recreate interface after stale socket: %w", createErr)
		}
		return c.getInterface(id)
	}

	return nil, fmt.Errorf("device error: %w", err)
}

func isStaleAWGSocketError(err error) bool {
	return errors.Is(err, unix.ECONNREFUSED)
}

func shouldTryAWG(awgMode string, needsAWG bool) bool {
	return awgMode == config.AWGModeAlways ||
		(awgMode == config.AWGModeAuto && needsAWG)
}

// getDeviceWithAWGFallback returns the WireGuard device for name, falling
// back to a direct AmneziaWG UAPI socket read when the kernel netlink
// client in wgctrl-go rejects the device with a non-`os.ErrNotExist` error.
//
// Background: wgctrl-go's Linux client tries the kernel netlink first, and
// only falls through to the userspace (UAPI) client when the kernel
// returns `os.ErrNotExist` (ENODEV). For an AWG TUN device the kernel's
// wireguard genetlink family does not recognise the interface and returns
// a different error (typically ENOTSUP / EBUSY), so the wrapper bails out
// with an empty device — and the dashboard shows zero peers / no
// handshake / no traffic for the AWG interface.
//
// Fallback policy:
//   - If the kernel client returns the device directly, use it.
//   - If the kernel client returns `os.ErrNotExist`, return that error
//     unchanged so the existing "interface not found" path applies.
//   - If the kernel client returns any other error AND a UAPI socket for
//     the interface exists AND AWG mode is not "never", read the device
//     dump straight from the UAPI socket.
//
// On fallback success, the original kernel error is wrapped so a
// forensic reader can see why we deviated. On fallback failure, the
// original error is returned (the UAPI path didn't make things worse).
func (c LocalController) getDeviceWithAWGFallback(name string) (*wgtypes.Device, error) {
	dev, err := c.wg.Device(name)
	if err == nil {
		return dev, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Non-NotExist error: maybe it's an AWG TUN that the kernel netlink
	// doesn't recognise. Try the UAPI socket fallback.
	if c.cfg.Backend.GetAWGMode() == config.AWGModeNever {
		return nil, err
	}
	if _, statErr := os.Stat(lowlevel.SocketPath(name)); statErr != nil {
		// No UAPI socket → not an AWG TUN (or AWG is down). Surface the
		// original kernel error unchanged.
		return nil, err
	}
	awgDev, awgErr := lowlevel.AWGDeviceFromUAPI(name)
	if awgErr != nil {
		slog.Warn("AWG UAPI fallback failed, returning original kernel error",
			"iface", name, "kernel_err", err, "uapi_err", awgErr)
		return nil, err
	}
	slog.Debug("AWG UAPI fallback succeeded for kernel-rejected device",
		"iface", name, "peers", len(awgDev.Peers))
	return awgDev, nil
}

func (c LocalController) getInterface(id domain.InterfaceIdentifier) (*domain.PhysicalInterface, error) {
	device, err := c.getDeviceWithAWGFallback(string(id))
	if err != nil {
		return nil, err
	}

	pi, err := c.convertWireGuardInterface(device)
	return &pi, err
}

func (c LocalController) createLowLevelInterface(id domain.InterfaceIdentifier, needsAWG bool) error {
	awgMode := c.cfg.Backend.GetAWGMode()

	// Decide whether to attempt amneziawg-go for this interface.
	//
	// Contract per awg_mode:
	//   - awg_mode=always → ALWAYS try amneziawg-go, even for plain WG
	//     interfaces (the user explicitly asked for userspace AWG for
	//     EVERY interface on this backend).
	//   - awg_mode=auto   → try amneziawg-go ONLY when the interface
	//     itself has AWG obfuscation enabled / configured. Plain WG
	//     interfaces stay on the kernel module to avoid creating a TUN
	//     device that they don't need.
	//   - awg_mode=never  → never start amneziawg-go, kernel WG only.
	//
	// needsAWG (from the PhysicalInterface) is independent of awg_mode;
	// it reflects the *interface's* requirements, not the backend's
	// policy. The two combine to determine shouldTryAWG.
	shouldTryAWG := shouldTryAWG(awgMode, needsAWG)

	if shouldTryAWG {
		awgErr := lowlevel.StartAWGProcess(string(id))
		if awgErr == nil {
			return nil // amneziawg-go created the TUN device + UAPI socket
		}
		// awg_mode=always: never silently downgrade to kernel WG. The user
		// explicitly asked for AWG and we couldn't honour that.
		if awgMode == config.AWGModeAlways {
			slog.Error("awg_mode=always but amneziawg-go is unavailable; refusing to fall back to kernel WireGuard",
				"iface", id, "error", awgErr)
			return fmt.Errorf("awg_mode=always: amneziawg-go unavailable for %s: %w", id, awgErr)
		}
		// awg_mode=auto: amneziawg-go failed AND the interface requires
		// AWG obfuscation. Surface the error rather than silently
		// dropping the obfuscation and using kernel WG (silent downgrade).
		if needsAWG {
			slog.Error("awg_mode=auto and interface requires AWG obfuscation, but amneziawg-go is unavailable; refusing to fall back to kernel WireGuard",
				"iface", id, "error", awgErr)
			return fmt.Errorf("awg_mode=auto but interface %s requires AWG and amneziawg-go unavailable: %w", id, awgErr)
		}
		// awg_mode=auto and needsAWG=false: we shouldn't actually reach
		// here (shouldTryAWG was false in that case), but if we do,
		// fall through to kernel WG.
		slog.Debug("amneziawg-go not available, falling back to kernel WireGuard", "iface", id)
	}

	link := &netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			Name: string(id),
		},
		LinkType: "wireguard",
	}
	err := c.nl.LinkAdd(link)
	if err != nil {
		return fmt.Errorf("link add failed: %w", err)
	}

	return nil
}

func (c LocalController) updateLowLevelInterface(pi *domain.PhysicalInterface) error {
	link, err := c.nl.LinkByName(string(pi.Identifier))
	if err != nil {
		return err
	}
	if pi.Mtu != 0 {
		if err := c.nl.LinkSetMTU(link, pi.Mtu); err != nil {
			return fmt.Errorf("mtu error: %w", err)
		}
	}

	for _, addr := range pi.Addresses {
		err := c.nl.AddrReplace(link, addr.NetlinkAddr())
		if err != nil {
			return fmt.Errorf("failed to set ip %s: %w", addr.String(), err)
		}
	}

	// Remove unwanted IP addresses
	rawAddresses, err := c.nl.AddrList(link)
	if err != nil {
		return fmt.Errorf("failed to fetch interface ips: %w", err)
	}
	for _, rawAddr := range rawAddresses {
		netlinkAddr := domain.CidrFromNetlinkAddr(rawAddr)
		remove := true
		for _, addr := range pi.Addresses {
			if addr == netlinkAddr {
				remove = false
				break
			}
		}

		if !remove {
			continue
		}

		err := c.nl.AddrDel(link, &rawAddr)
		if err != nil {
			return fmt.Errorf("failed to remove deprecated ip %s: %w", netlinkAddr.String(), err)
		}
	}

	// Update link state
	if pi.DeviceUp {
		if err := c.nl.LinkSetUp(link); err != nil {
			return fmt.Errorf("failed to bring up device: %w", err)
		}
	} else {
		if err := c.nl.LinkSetDown(link); err != nil {
			return fmt.Errorf("failed to bring down device: %w", err)
		}
	}

	return nil
}

func (c LocalController) updateWireGuardInterface(pi *domain.PhysicalInterface) error {
	pKey, err := wgtypes.NewKey(pi.KeyPair.GetPrivateKeyBytes())
	if err != nil {
		return err
	}

	var fwMark *int
	if pi.FirewallMark != 0 {
		intFwMark := int(pi.FirewallMark)
		fwMark = &intFwMark
	}

	cfg := wgtypes.Config{
		PrivateKey:   &pKey,
		ListenPort:   &pi.ListenPort,
		FirewallMark: fwMark,
		ReplacePeers: false,
	}

	// Apply AmneziaWG obfuscation parameters directly via wgtypes.Config.
	// Jipok/wgctrl-go sends them natively through the UAPI.
	// Note: wgtypes.Config uses *string for H1-H4 (UAPI protocol is string-based),
	// so we convert our uint32 values via fmt.Sprintf.
	//
	// Honour the awg_mode=never contract: when the backend is configured
	// for plain kernel WireGuard, we MUST NOT pass AWG parameters to
	// wgctrl, even if the interface model has them populated. Otherwise
	// a Jipok wgctrl-go built with AWG support would forward them to the
	// kernel UAPI socket, where they would either be rejected as unknown
	// keys or, worse, silently accepted by a custom UAPI handler.
	//
	// Inside the non-never branch we additionally gate on
	// pi.EmitAWG() — the single source of truth for "should we emit
	// AWG?" — so a stale AWG params field with AWGEnabled=false
	// cannot leak into the kernel UAPI either.
	if awgMode := c.cfg.Backend.GetAWGMode(); awgMode != config.AWGModeNever && pi.EmitAWG() {
		awgParams, _ := pi.GetAWGParams()
		cfg.Jc = &awgParams.Jc
		cfg.Jmin = &awgParams.Jmin
		cfg.Jmax = &awgParams.Jmax
		cfg.S1 = &awgParams.S1
		cfg.S2 = &awgParams.S2
		cfg.S3 = &awgParams.S3
		cfg.S4 = &awgParams.S4
		h1 := fmt.Sprintf("%d", awgParams.H1)
		h2 := fmt.Sprintf("%d", awgParams.H2)
		h3 := fmt.Sprintf("%d", awgParams.H3)
		h4 := fmt.Sprintf("%d", awgParams.H4)
		cfg.H1 = &h1
		cfg.H2 = &h2
		cfg.H3 = &h3
		cfg.H4 = &h4
	} else {
		// awg_mode=never OR pi.EmitAWG()=false: ensure no AWG fields
		// leak into wgtypes.Config. This is the safe path even though
		// the cfg literal above starts with all nil pointers; defensive
		// in case future refactors accidentally populate these fields.
		cfg.Jc = nil
		cfg.Jmin = nil
		cfg.Jmax = nil
		cfg.S1, cfg.S2, cfg.S3, cfg.S4 = nil, nil, nil, nil
		cfg.H1, cfg.H2, cfg.H3, cfg.H4 = nil, nil, nil, nil
	}

	err = c.wg.ConfigureDevice(string(pi.Identifier), cfg)
	if err != nil {
		return err
	}

	return nil
}

func (c LocalController) DeleteInterface(_ context.Context, id domain.InterfaceIdentifier) error {
	if err := c.deleteLowLevelInterface(id); err != nil {
		return err
	}

	return nil
}

func (c LocalController) deleteLowLevelInterface(id domain.InterfaceIdentifier) error {
	// Stop amneziawg-go process if running (best-effort).
	// Runs before netlink.LinkDel to ensure clean teardown.
	_ = lowlevel.StopAWGProcess(string(id))

	link, err := c.nl.LinkByName(string(id))
	if err != nil {
		var linkNotFoundError netlink.LinkNotFoundError
		if errors.As(err, &linkNotFoundError) {
			return nil // ignore not found error
		}
		return fmt.Errorf("unable to find low level interface: %w", err)
	}

	err = c.nl.LinkDel(link)
	if err != nil {
		return fmt.Errorf("failed to delete low level interface: %w", err)
	}

	return nil
}

func (c LocalController) SavePeer(
	_ context.Context,
	deviceId domain.InterfaceIdentifier,
	id domain.PeerIdentifier,
	updateFunc func(pp *domain.PhysicalPeer) (*domain.PhysicalPeer, error),
) error {
	// For AmneziaWG interfaces, use direct UAPI communication to bypass
	// wgctrl-go's incorrect interface public_key → peer parsing.
	awgSock := lowlevel.SocketPath(string(deviceId))
	if _, statErr := os.Stat(awgSock); statErr == nil && c.cfg.Backend.GetAWGMode() != config.AWGModeNever {
		slog.Debug("using direct AWG UAPI for peer save", "iface", deviceId, "peer", id, "socket", awgSock)
		return c.saveAWGPeer(deviceId, id, updateFunc)
	}
	slog.Debug("fallback to wgctrl for peer save", "iface", deviceId, "peer", id)

	physicalPeer, err := c.getOrCreatePeer(deviceId, id)
	if err != nil {
		return err
	}

	physicalPeer, err = updateFunc(physicalPeer)
	if err != nil {
		return err
	}

	// Check if the peer is disabled by looking at the backend extras
	// For local controller, disabled peers should be deleted
	if physicalPeer.GetExtras() != nil {
		switch extras := physicalPeer.GetExtras().(type) {
		case domain.LocalPeerExtras:
			if extras.Disabled {
				// Delete the peer instead of updating it
				return c.deletePeer(deviceId, id)
			}
		}
	}

	if err := c.updatePeer(deviceId, physicalPeer); err != nil {
		return err
	}

	return nil
}

// saveAWGPeer manages peers on AmneziaWG interfaces via direct UAPI.
func (c LocalController) saveAWGPeer(
	deviceId domain.InterfaceIdentifier,
	id domain.PeerIdentifier,
	updateFunc func(pp *domain.PhysicalPeer) (*domain.PhysicalPeer, error),
) error {
	// Create empty physical peer, apply the update
	pp := &domain.PhysicalPeer{
		Identifier: id,
		KeyPair: domain.KeyPair{
			PublicKey: string(id),
		},
	}

	var err error
	pp, err = updateFunc(pp)
	if err != nil {
		return err
	}

	// Check if the peer is disabled
	if pp.GetExtras() != nil {
		switch extras := pp.GetExtras().(type) {
		case domain.LocalPeerExtras:
			if extras.Disabled {
				return lowlevel.RemoveAWGPeer(string(deviceId), string(id))
			}
		}
	}

	// Build allowed IPs from the peer
	allowedIPs := make([]string, 0, len(pp.AllowedIPs))
	for _, cidr := range pp.AllowedIPs {
		allowedIPs = append(allowedIPs, cidr.String())
	}

	// Build endpoint string from the peer's endpoint (if set)
	endpoint := ""
	if pp.Endpoint != "" {
		endpoint = pp.Endpoint
	}

	// Get preshared key (base64-encoded)
	psk := ""
	if pp.PresharedKey != "" {
		psk = string(pp.PresharedKey)
	}

	// Send via UAPI
	return lowlevel.SetAWGPeer(string(deviceId), lowlevel.AWGUAPIPeerConfig{
		PublicKey:    string(id),
		PresharedKey: psk,
		Endpoint:     endpoint,
		AllowedIPs:   allowedIPs,
		Keepalive:    pp.PersistentKeepalive,
	})
}

func (c LocalController) getOrCreatePeer(deviceId domain.InterfaceIdentifier, id domain.PeerIdentifier) (
	*domain.PhysicalPeer,
	error,
) {
	peer, err := c.getPeer(deviceId, id)
	if err == nil {
		return peer, nil // peer exists
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("peer error: %w", err) // unknown error
	}

	// create new peer
	err = c.wg.ConfigureDevice(string(deviceId), wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: id.ToPublicKey(),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("peer create error for %s: %w", id.ToPublicKey(), err)
	}

	peer, err = c.getPeer(deviceId, id)
	if err != nil {
		return nil, fmt.Errorf("peer error after create: %w", err)
	}
	return peer, nil
}

func (c LocalController) getPeer(deviceId domain.InterfaceIdentifier, id domain.PeerIdentifier) (
	*domain.PhysicalPeer,
	error,
) {
	if !id.IsPublicKey() {
		return nil, errors.New("invalid public key")
	}

	device, err := c.getDeviceWithAWGFallback(string(deviceId))
	if err != nil {
		return nil, err
	}

	publicKey := id.ToPublicKey()
	for _, peer := range device.Peers {
		if peer.PublicKey != publicKey {
			continue
		}

		peerModel, err := c.convertWireGuardPeer(&peer)
		return &peerModel, err
	}

	return nil, os.ErrNotExist
}

func (c LocalController) updatePeer(deviceId domain.InterfaceIdentifier, pp *domain.PhysicalPeer) error {
	cfg := wgtypes.PeerConfig{
		PublicKey:                   pp.GetPublicKey(),
		Remove:                      false,
		UpdateOnly:                  true, // true = only update if exists; create handled by getOrCreatePeer
		PresharedKey:                pp.GetPresharedKey(),
		Endpoint:                    pp.GetEndpointAddress(),
		PersistentKeepaliveInterval: pp.GetPersistentKeepaliveTime(),
		ReplaceAllowedIPs:           true,
		AllowedIPs:                  pp.GetAllowedIPs(),
	}

	err := c.wg.ConfigureDevice(string(deviceId), wgtypes.Config{ReplacePeers: false, Peers: []wgtypes.PeerConfig{cfg}})
	if err != nil {
		return err
	}

	return nil
}

func (c LocalController) DeletePeer(
	_ context.Context,
	deviceId domain.InterfaceIdentifier,
	id domain.PeerIdentifier,
) error {
	if !id.IsPublicKey() {
		return errors.New("invalid public key")
	}

	err := c.deletePeer(deviceId, id)
	if err != nil {
		return err
	}

	return nil
}

func (c LocalController) deletePeer(deviceId domain.InterfaceIdentifier, id domain.PeerIdentifier) error {
	cfg := wgtypes.PeerConfig{
		PublicKey: id.ToPublicKey(),
		Remove:    true,
	}

	err := c.wg.ConfigureDevice(string(deviceId), wgtypes.Config{ReplacePeers: false, Peers: []wgtypes.PeerConfig{cfg}})
	if err != nil {
		return err
	}

	return nil
}

// endregion wireguard-related

// region wg-quick-related

func (c LocalController) ExecuteInterfaceHook(
	_ context.Context,
	id domain.InterfaceIdentifier,
	hookCmd string,
) error {
	if hookCmd == "" {
		return nil
	}

	slog.Debug("executing interface hook", "interface", id, "hook", hookCmd)
	err := c.exec(hookCmd, id)
	if err != nil {
		return fmt.Errorf("failed to exec hook: %w", err)
	}

	return nil
}

func (c LocalController) SetDNS(_ context.Context, id domain.InterfaceIdentifier, dnsStr, dnsSearchStr string) error {
	if dnsStr == "" && dnsSearchStr == "" {
		return nil
	}

	dnsServers := internal.SliceString(dnsStr)
	dnsSearchDomains := internal.SliceString(dnsSearchStr)

	dnsCommand := "resolvconf -a %resPref%i -m 0 -x"
	dnsCommandInput := make([]string, 0, len(dnsServers)+len(dnsSearchDomains))

	for _, dnsServer := range dnsServers {
		dnsCommandInput = append(dnsCommandInput, fmt.Sprintf("nameserver %s", dnsServer))
	}
	for _, searchDomain := range dnsSearchDomains {
		dnsCommandInput = append(dnsCommandInput, fmt.Sprintf("search %s", searchDomain))
	}

	err := c.exec(dnsCommand, id, dnsCommandInput...)
	if err != nil {
		return fmt.Errorf(
			"failed to set dns settings (is resolvconf available?, for systemd create this symlink: ln -s /usr/bin/resolvectl /usr/local/bin/resolvconf): %w",
			err,
		)
	}

	return nil
}

func (c LocalController) UnsetDNS(_ context.Context, id domain.InterfaceIdentifier, _, _ string) error {
	dnsCommand := "resolvconf -d %resPref%i -f"

	err := c.exec(dnsCommand, id)
	if err != nil {
		return fmt.Errorf("failed to unset dns settings: %w", err)
	}

	return nil
}

func (c LocalController) replaceCommandPlaceHolders(command string, interfaceId domain.InterfaceIdentifier) string {
	command = strings.ReplaceAll(command, "%resPref", c.resolvConfIfacePrefix)
	return strings.ReplaceAll(command, "%i", string(interfaceId))
}

func (c LocalController) exec(command string, interfaceId domain.InterfaceIdentifier, stdin ...string) error {
	commandWithInterfaceName := c.replaceCommandPlaceHolders(command, interfaceId)
	cmd := exec.Command(c.shellCmd, "-ce", commandWithInterfaceName)
	if len(stdin) > 0 {
		b := &bytes.Buffer{}
		for _, ln := range stdin {
			if _, err := fmt.Fprint(b, ln+"\n"); err != nil {
				return err
			}
		}
		cmd.Stdin = b
	}
	out, err := cmd.CombinedOutput() // execute and wait for output
	if err != nil {
		slog.Warn("failed to executed shell command",
			"command", commandWithInterfaceName, "stdin", stdin, "output", string(out), "error", err)
		return fmt.Errorf("failed to execute shell command %s: %w", commandWithInterfaceName, err)
	}
	slog.Debug("executed shell command",
		"command", commandWithInterfaceName,
		"output", string(out))
	return nil
}

// endregion wg-quick-related

// region routing-related

// SetRoutes sets the routes for the given interface. If no routes are provided, the function is a no-op.
func (c LocalController) SetRoutes(_ context.Context, info domain.RoutingTableInfo) error {
	interfaceId := info.Interface.Identifier
	slog.Debug("setting linux routes", "interface", interfaceId, "table", info.Table, "fwMark", info.FwMark,
		"cidrs", info.AllowedIps)

	link, err := c.nl.LinkByName(string(interfaceId))
	if err != nil {
		return fmt.Errorf("failed to find physical link for %s: %w", interfaceId, err)
	}

	cidrsV4, cidrsV6 := domain.CidrsPerFamily(info.AllowedIps)
	realTable, realFwMark, err := c.getOrCreateRoutingTableAndFwMark(link, info.Table, info.FwMark)
	if err != nil {
		return fmt.Errorf("failed to get or create routing table and fwmark for %s: %w", interfaceId, err)
	}
	wgDev, err := c.wg.Device(string(interfaceId))
	if err != nil {
		return fmt.Errorf("failed to get wg device for %s: %w", interfaceId, err)
	}
	currentFwMark := wgDev.FirewallMark
	if int(realFwMark) != currentFwMark {
		slog.Debug("updating fwmark for interface", "interface", interfaceId, "oldFwMark", currentFwMark,
			"newFwMark", realFwMark, "oldTable", info.Table, "newTable", realTable)
		if err := c.updateFwMarkOnInterface(interfaceId, int(realFwMark)); err != nil {
			return fmt.Errorf("failed to update fwmark for interface %s to %d: %w", interfaceId, realFwMark, err)
		}
	}

	if err := c.setRoutesForFamily(interfaceId, link, netlink.FAMILY_V4, realTable, realFwMark, cidrsV4); err != nil {
		return fmt.Errorf("failed to set v4 routes: %w", err)
	}
	if err := c.setRoutesForFamily(interfaceId, link, netlink.FAMILY_V6, realTable, realFwMark, cidrsV6); err != nil {
		return fmt.Errorf("failed to set v6 routes: %w", err)
	}

	// HACK: setRoutesForFamily removes kernel-created connected routes (scope=link,
	// type=unicast) from the main routing table. This affects ALL interfaces:
	//   - AWG TUN (amneziawg-go) — no automatic connected route from kernel
	//   - Plain WG kernel — connected route gets deleted by route cleanup logic
	// Re-add the connected route(s) here after route management completes.
	slog.Debug("restoring connected route(s) for interface", "iface", interfaceId)
	link, linkErr := c.nl.LinkByName(string(interfaceId))
	if linkErr != nil {
		return fmt.Errorf("failed to get link for connected route: %w", linkErr)
	}
	for _, addr := range info.Interface.Addresses {
		network := addr.NetworkAddr()
		if routeErr := c.nl.RouteAdd(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       network.IpNet(),
			Scope:     netlink.SCOPE_LINK,
		}); routeErr != nil && !errors.Is(routeErr, unix.EEXIST) {
			return fmt.Errorf("failed to add connected route %s: %w", network.String(), routeErr)
		}
	}

	return nil
}

func (c LocalController) setRoutesForFamily(
	interfaceId domain.InterfaceIdentifier,
	link netlink.Link,
	family int,
	table int,
	fwMark uint32,
	cidrs []domain.Cidr,
) error {
	// first create or update the routes
	for _, cidr := range cidrs {
		err := c.nl.RouteReplace(&netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       cidr.IpNet(),
			Table:     table,
			Scope:     unix.RT_SCOPE_LINK,
			Type:      unix.RTN_UNICAST,
		})
		if err != nil {
			return fmt.Errorf("failed to add/update route %s on table %d for interface %s: %w",
				cidr.String(), table, interfaceId, err)
		}
	}

	// next remove old routes
	rawRoutes, err := c.nl.RouteListFiltered(family, &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Table:     unix.RT_TABLE_UNSPEC, // all tables
		Scope:     unix.RT_SCOPE_LINK,
		Type:      unix.RTN_UNICAST,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_TYPE|netlink.RT_FILTER_OIF)
	if err != nil {
		return fmt.Errorf("failed to fetch raw routes for interface %s and family-id %d: %w",
			interfaceId, family, err)
	}
	for _, rawRoute := range rawRoutes {
		if rawRoute.Dst == nil { // handle default route
			var netlinkAddr domain.Cidr
			if family == netlink.FAMILY_V4 {
				netlinkAddr, _ = domain.CidrFromString("0.0.0.0/0")
			} else {
				netlinkAddr, _ = domain.CidrFromString("::/0")
			}
			rawRoute.Dst = netlinkAddr.IpNet()
		}

		route := domain.CidrFromIpNet(*rawRoute.Dst)
		if slices.Contains(cidrs, route) {
			continue
		}

		if err := c.nl.RouteDel(&rawRoute); err != nil {
			return fmt.Errorf("failed to remove deprecated route %s from interface %s: %w", route, interfaceId, err)
		}
	}

	// next, update route rules for normal routes
	if table == 0 {
		return nil // no need to update route rules as we are using the default table
	}
	existingRules, err := c.nl.RuleList(family)
	if err != nil {
		return fmt.Errorf("failed to get existing rules for family-id %d: %w", family, err)
	}
	ruleExists := slices.ContainsFunc(existingRules, func(rule netlink.Rule) bool {
		return rule.Mark == fwMark && rule.Table == table
	})
	if !ruleExists {
		if err := c.nl.RuleAdd(&netlink.Rule{
			Family:            family,
			Table:             table,
			Mark:              fwMark,
			Invert:            false,
			SuppressIfgroup:   -1,
			SuppressPrefixlen: -1,
			Priority:          c.getRulePriority(existingRules),
			Mask:              nil,
			Goto:              -1,
			Flow:              -1,
		}); err != nil {
			return fmt.Errorf("failed to setup rule for fwmark %d and table %d for family-id %d: %w",
				fwMark, table, family, err)
		}
	}
	mainRuleExists := slices.ContainsFunc(existingRules, func(rule netlink.Rule) bool {
		return rule.SuppressPrefixlen == 0 && rule.Table == unix.RT_TABLE_MAIN
	})
	if !mainRuleExists && domain.ContainsDefaultRoute(cidrs) {
		err = c.nl.RuleAdd(&netlink.Rule{
			Family:            family,
			Table:             unix.RT_TABLE_MAIN,
			SuppressIfgroup:   -1,
			SuppressPrefixlen: 0,
			Priority:          c.getMainRulePriority(existingRules),
			Mark:              0,
			Mask:              nil,
			Goto:              -1,
			Flow:              -1,
		})
	}

	// finally, clean up extra main rules - only one rule is allowed
	existingRules, err = c.nl.RuleList(family)
	if err != nil {
		return fmt.Errorf("failed to get existing main rules for family-id %d: %w", family, err)
	}
	mainRuleCount := 0
	for _, rule := range existingRules {
		if rule.SuppressPrefixlen == 0 && rule.Table == unix.RT_TABLE_MAIN {
			mainRuleCount++
		}
		if mainRuleCount > 1 {
			if err := c.nl.RuleDel(&rule); err != nil {
				return fmt.Errorf("failed to remove extra main rule for family-id %d: %w", family, err)
			}
		}
	}

	return nil
}

func (c LocalController) getOrCreateRoutingTableAndFwMark(
	link netlink.Link,
	tableIn int,
	fwMarkIn uint32,
) (
	table int,
	fwmark uint32,
	err error,
) {
	table = tableIn
	fwmark = fwMarkIn

	if fwmark == 0 {
		// generate a new (temporary) firewall mark based on the interface index
		fwmark = uint32(c.cfg.Advanced.RouteTableOffset + link.Attrs().Index)
	}
	if table == 0 {
		table = int(fwmark) // generate a new routing table base on interface index
	}
	return
}

func (c LocalController) updateFwMarkOnInterface(interfaceId domain.InterfaceIdentifier, fwMark int) error {
	// apply the new fwmark to the wireguard interface
	err := c.wg.ConfigureDevice(string(interfaceId), wgtypes.Config{
		FirewallMark: &fwMark,
	})
	if err != nil {
		return fmt.Errorf("failed to update fwmark of interface %s to: %d: %w", interfaceId, fwMark, err)
	}

	return nil
}

func (c LocalController) getMainRulePriority(existingRules []netlink.Rule) int {
	prio := c.cfg.Advanced.RulePrioOffset
	for {
		isFresh := true
		for _, existingRule := range existingRules {
			if existingRule.Priority == prio {
				isFresh = false
				break
			}
		}
		if isFresh {
			break
		} else {
			prio++
		}
	}
	return prio
}

func (c LocalController) getRulePriority(existingRules []netlink.Rule) int {
	prio := 32700 // linux main rule has a prio of 32766
	for {
		isFresh := true
		for _, existingRule := range existingRules {
			if existingRule.Priority == prio {
				isFresh = false
				break
			}
		}
		if isFresh {
			break
		} else {
			prio--
		}
	}
	return prio
}

// RemoveRoutes removes the routes for the given interface. If no routes are provided, the function is a no-op.
func (c LocalController) RemoveRoutes(_ context.Context, info domain.RoutingTableInfo) error {
	interfaceId := info.Interface.Identifier
	slog.Debug("removing linux routes", "interface", interfaceId, "table", info.Table, "fwMark", info.FwMark,
		"cidrs", info.AllowedIps)

	wgDev, err := c.wg.Device(string(interfaceId))
	if err != nil {
		slog.Debug("wg device already removed, route cleanup might be incomplete", "interface", interfaceId)
		wgDev = nil
	}
	link, err := c.nl.LinkByName(string(interfaceId))
	if err != nil {
		slog.Debug("physical link already removed, route cleanup might be incomplete", "interface", interfaceId)
		link = nil
	}

	// Bug 2 fix: при удалении AWG-интерфейса физический netlink-линк может
	// быть уже снесён к моменту route cleanup (link == nil). В этом случае
	// маршруты снимать бессмысленно — ядро само убирает их при удалении
	// линка — а раньше код безусловно дёргал link.Attrs().Index и падал в
	// nil pointer dereference, из-за чего весь сервис падал в panic при
	// удалении AWG-интерфейса. Ранний выход безопасен: для оставшегося
	// wgDev (если он есть) тоже нечего чистить — он исчезнет вместе с линком.
	if link == nil {
		slog.Debug("physical link already removed, skipping route cleanup",
			"interface", interfaceId)
		return nil
	}

	fwMark := info.FwMark
	if wgDev != nil && info.FwMark == 0 {
		fwMark = uint32(wgDev.FirewallMark)
	}
	table := info.Table
	if wgDev != nil && info.Table == 0 {
		table = wgDev.FirewallMark // use the fwMark as table, this is the default behavior
	}
	linkIndex := -1
	if link != nil {
		linkIndex = link.Attrs().Index
	}

	cidrsV4, cidrsV6 := domain.CidrsPerFamily(info.AllowedIps)
	realTable, realFwMark, err := c.getOrCreateRoutingTableAndFwMark(link, table, fwMark)
	if err != nil {
		return fmt.Errorf("failed to get or create routing table and fwmark for %s: %w", interfaceId, err)
	}

	if linkIndex > 0 {
		err = c.removeRoutesForFamily(interfaceId, link, netlink.FAMILY_V4, realTable, realFwMark, cidrsV4)
		if err != nil {
			return fmt.Errorf("failed to remove v4 routes: %w", err)
		}
		err = c.removeRoutesForFamily(interfaceId, link, netlink.FAMILY_V6, realTable, realFwMark, cidrsV6)
		if err != nil {
			return fmt.Errorf("failed to remove v6 routes: %w", err)
		}
	}

	if table > 0 {
		err = c.removeRouteRulesForTable(netlink.FAMILY_V4, realTable)
		if err != nil {
			return fmt.Errorf("failed to remove v4 route rules for %s: %w", interfaceId, err)
		}
		err = c.removeRouteRulesForTable(netlink.FAMILY_V6, realTable)
		if err != nil {
			return fmt.Errorf("failed to remove v6 route rules for %s: %w", interfaceId, err)
		}
	}

	return nil
}

func (c LocalController) removeRoutesForFamily(
	interfaceId domain.InterfaceIdentifier,
	link netlink.Link,
	family int,
	table int,
	fwMark uint32,
	cidrs []domain.Cidr,
) error {
	// first remove all rules
	existingRules, err := c.nl.RuleList(family)
	if err != nil {
		return fmt.Errorf("failed to get existing rules for family %d: %w", family, err)
	}
	for _, existingRule := range existingRules {
		if fwMark == existingRule.Mark && table == existingRule.Table {
			existingRule.Family = family // set family, somehow the RuleList method does not populate the family field
			if err := c.nl.RuleDel(&existingRule); err != nil {
				return fmt.Errorf("failed to delete old fwmark rule: %w", err)
			}
		}
	}

	// next remove all routes
	rawRoutes, err := c.nl.RouteListFiltered(family, &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Table:     unix.RT_TABLE_UNSPEC, // all tables
		Scope:     unix.RT_SCOPE_LINK,
		Type:      unix.RTN_UNICAST,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_TYPE|netlink.RT_FILTER_OIF)
	if err != nil {
		return fmt.Errorf("failed to fetch raw routes for interface %s and family-id %d: %w",
			interfaceId, family, err)
	}
	for _, rawRoute := range rawRoutes {
		if rawRoute.Dst == nil { // handle default route
			var netlinkAddr domain.Cidr
			if family == netlink.FAMILY_V4 {
				netlinkAddr, _ = domain.CidrFromString("0.0.0.0/0")
			} else {
				netlinkAddr, _ = domain.CidrFromString("::/0")
			}
			rawRoute.Dst = netlinkAddr.IpNet()
		}

		if rawRoute.Table != table {
			continue // ignore routes from other tables
		}

		route := domain.CidrFromIpNet(*rawRoute.Dst)
		if !slices.Contains(cidrs, route) {
			continue // only remove routes that were previously added
		}

		if err := c.nl.RouteDel(&rawRoute); err != nil {
			return fmt.Errorf("failed to remove old route %s from interface %s: %w", route, interfaceId, err)
		}
	}

	return nil
}

func (c LocalController) removeRouteRulesForTable(
	family int,
	table int,
) error {
	existingRules, err := c.nl.RuleList(family)
	if err != nil {
		return fmt.Errorf("failed to get existing route rules for family-id %d: %w", family, err)
	}
	for _, existingRule := range existingRules {
		if existingRule.Table == table {
			err := c.nl.RuleDel(&existingRule)
			if err != nil {
				return fmt.Errorf("failed to delete old rule for table %d and family-id %d: %w", table, family, err)
			}
		}
	}
	return nil
}

// endregion routing-related

// region statistics-related

func (c LocalController) PingAddresses(
	ctx context.Context,
	addr string,
) (*domain.PingerResult, error) {
	pinger, err := probing.NewPinger(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate pinger for %s: %w", addr, err)
	}

	checkCount := 1
	pinger.SetPrivileged(!c.cfg.Statistics.PingUnprivileged)
	pinger.Count = checkCount
	pinger.Timeout = 2 * time.Second
	err = pinger.RunWithContext(ctx) // Blocks until finished.
	if err != nil {
		return nil, fmt.Errorf("failed to ping %s: %w", addr, err)
	}

	stats := pinger.Statistics()

	return &domain.PingerResult{
		PacketsRecv: stats.PacketsRecv,
		PacketsSent: stats.PacketsSent,
		Rtts:        stats.Rtts,
	}, nil
}

// endregion statistics-related

// ensureAWGConnectedRoute adds connected routes for AWG (TUN) interfaces.
// amneziawg-go creates TUN devices via tun.CreateTUN() which, unlike kernel
// WireGuard interfaces or ip tuntap add, doesn't trigger the kernel to add
// automatic connected routes. Without this, return traffic from the AWG
// interface has no route and ping/IP traffic fails.
func (c LocalController) ensureAWGConnectedRoute(pi *domain.PhysicalInterface) error {
	if _, isAWG := pi.GetAWGParams(); !isAWG {
		return nil // not an AWG interface, nothing to do
	}

	link, err := c.nl.LinkByName(string(pi.Identifier))
	if err != nil {
		return fmt.Errorf("failed to get link for AWG route: %w", err)
	}

	for _, addr := range pi.Addresses {
		if !addr.IsV4() {
			continue // only IPv4 for now
		}
		network := addr.NetworkAddr()
		// Matches `ip route add <network>/<mask> dev <iface> scope link`
		route := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       network.IpNet(),
			Scope:     netlink.SCOPE_LINK,
		}
		if err := c.nl.RouteAdd(route); err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue // already exists, fine
			}
			return fmt.Errorf("failed to add AWG connected route %s: %w", network.String(), err)
		}
	}
	return nil
}
