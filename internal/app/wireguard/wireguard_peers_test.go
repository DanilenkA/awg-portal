package wireguard

import (
	"context"
	"testing"

	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
)

// --- Test mocks ---

type mockBus struct{}

func (f *mockBus) Publish(topic string, args ...any)            {}
func (f *mockBus) Subscribe(topic string, fn interface{}) error { return nil }

type mockController struct{}

func (f *mockController) GetId() domain.InterfaceBackend { return "local" }
func (f *mockController) GetInterfaces(_ context.Context) ([]domain.PhysicalInterface, error) {
	return nil, nil
}
func (f *mockController) GetInterface(_ context.Context, id domain.InterfaceIdentifier) (
	*domain.PhysicalInterface,
	error,
) {
	return &domain.PhysicalInterface{Identifier: id}, nil
}
func (f *mockController) GetPeers(_ context.Context, _ domain.InterfaceIdentifier) ([]domain.PhysicalPeer, error) {
	return nil, nil
}
func (f *mockController) SaveInterface(
	_ context.Context,
	_ domain.InterfaceIdentifier,
	updateFunc func(pi *domain.PhysicalInterface) (*domain.PhysicalInterface, error),
) error {
	_, _ = updateFunc(&domain.PhysicalInterface{})
	return nil
}
func (f *mockController) DeleteInterface(_ context.Context, _ domain.InterfaceIdentifier) error {
	return nil
}
func (f *mockController) SavePeer(
	_ context.Context,
	_ domain.InterfaceIdentifier,
	_ domain.PeerIdentifier,
	updateFunc func(pp *domain.PhysicalPeer) (*domain.PhysicalPeer, error),
) error {
	_, _ = updateFunc(&domain.PhysicalPeer{})
	return nil
}
func (f *mockController) DeletePeer(_ context.Context, _ domain.InterfaceIdentifier, _ domain.PeerIdentifier) error {
	return nil
}
func (f *mockController) PingAddresses(_ context.Context, _ string) (*domain.PingerResult, error) {
	return nil, nil
}

type mockDB struct {
	savedPeers map[domain.PeerIdentifier]*domain.Peer
	iface      *domain.Interface
	interfaces []domain.Interface
	users      []domain.User
}

func (f *mockDB) GetInterface(ctx context.Context, id domain.InterfaceIdentifier) (*domain.Interface, error) {
	if f.iface != nil && f.iface.Identifier == id {
		return f.iface, nil
	}
	return &domain.Interface{Identifier: id}, nil
}
func (f *mockDB) GetInterfaceAndPeers(ctx context.Context, id domain.InterfaceIdentifier) (
	*domain.Interface,
	[]domain.Peer,
	error,
) {
	return f.iface, nil, nil
}
func (f *mockDB) GetPeersStats(ctx context.Context, ids ...domain.PeerIdentifier) ([]domain.PeerStatus, error) {
	return nil, nil
}
func (f *mockDB) GetAllInterfaces(ctx context.Context) ([]domain.Interface, error) {
	if f.interfaces != nil {
		return f.interfaces, nil
	}
	if f.iface != nil {
		return []domain.Interface{*f.iface}, nil
	}
	return nil, nil
}
func (f *mockDB) GetInterfaceIps(ctx context.Context) (map[domain.InterfaceIdentifier][]domain.Cidr, error) {
	return nil, nil
}
func (f *mockDB) SaveInterface(
	ctx context.Context,
	id domain.InterfaceIdentifier,
	updateFunc func(in *domain.Interface) (*domain.Interface, error),
) error {
	if f.iface == nil {
		f.iface = &domain.Interface{Identifier: id}
	}
	var err error
	f.iface, err = updateFunc(f.iface)
	return err
}
func (f *mockDB) DeleteInterface(ctx context.Context, id domain.InterfaceIdentifier) error {
	return nil
}
func (f *mockDB) GetInterfacePeers(ctx context.Context, id domain.InterfaceIdentifier) ([]domain.Peer, error) {
	return nil, nil
}
func (f *mockDB) GetUserPeers(ctx context.Context, id domain.UserIdentifier) ([]domain.Peer, error) {
	return nil, nil
}
func (f *mockDB) SavePeer(
	ctx context.Context,
	id domain.PeerIdentifier,
	updateFunc func(in *domain.Peer) (*domain.Peer, error),
) error {
	if f.savedPeers == nil {
		f.savedPeers = make(map[domain.PeerIdentifier]*domain.Peer)
	}
	existing := f.savedPeers[id]
	if existing == nil {
		existing = &domain.Peer{Identifier: id}
	}
	updated, err := updateFunc(existing)
	if err != nil {
		return err
	}
	f.savedPeers[updated.Identifier] = updated
	return nil
}
func (f *mockDB) DeletePeer(ctx context.Context, id domain.PeerIdentifier) error { return nil }
func (f *mockDB) GetPeer(ctx context.Context, id domain.PeerIdentifier) (*domain.Peer, error) {
	return nil, domain.ErrNotFound
}
func (f *mockDB) GetUsedIpsPerSubnet(ctx context.Context, subnets []domain.Cidr) (
	map[domain.Cidr][]domain.Cidr,
	error,
) {
	return map[domain.Cidr][]domain.Cidr{}, nil
}
func (f *mockDB) GetUser(ctx context.Context, id domain.UserIdentifier) (*domain.User, error) {
	return &domain.User{
		Identifier: id,
		IsAdmin:    false,
	}, nil
}
func (f *mockDB) GetAllUsers(ctx context.Context) ([]domain.User, error) {
	return f.users, nil
}

// --- Test ---

func TestCreatePeer_SetsIdentifier_FromPublicKey(t *testing.T) {
	// Arrange
	cfg := &config.Config{}
	cfg.Core.SelfProvisioningAllowed = true
	cfg.Core.EditableKeys = true
	cfg.Advanced.LimitAdditionalUserPeers = 0

	bus := &mockBus{}

	// Prepare a controller manager with our mock controller
	ctrlMgr := &ControllerManager{
		controllers: map[domain.InterfaceBackend]backendInstance{
			config.LocalBackendName: {Implementation: &mockController{}},
		},
	}

	db := &mockDB{iface: &domain.Interface{Identifier: "wg0", Type: domain.InterfaceTypeServer}}

	m := Manager{
		cfg: cfg,
		bus: bus,
		db:  db,
		wg:  ctrlMgr,
	}

	userId := domain.UserIdentifier("user@example.com")
	// IsAdmin=true on purpose: per the current CreatePeer contract
	// (introduced in commit ec24411 "fix: генерация ключей пира при
	// создании через API"), the only path that sets peer.Identifier
	// from peer.Interface.PublicKey without going through PreparePeer
	// (and therefore without re-deriving the Identifier from a fresh
	// server-generated key) is the admin branch in CreatePeer:
	//
	//   } else {
	//       // Admin provided keys — ensure Identifier matches PublicKey
	//       peer.Identifier = domain.PeerIdentifier(peer.Interface.PublicKey)
	//   }
	//
	// This branch is reachable only when BOTH conditions hold:
	//   1. sessionUser.IsAdmin == true
	//   2. peer.Interface.PrivateKey != ""
	//
	// Non-admin users (self-provisioning) always go through PreparePeer,
	// which generates fresh keys server-side and ignores the caller's
	// PublicKey unless the caller also provides a matching PrivateKey.
	// That guard prevents key-substitution attacks by a non-admin user.
	// The old test (pre-ec24411) was written under the previous
	// contract where the Identifier was always derived from the
	// caller's PublicKey, regardless of role.
	ctx := domain.SetUserInfo(context.Background(), &domain.ContextUserInfo{Id: userId, IsAdmin: true})

	pubKey := "TEST_PUBLIC_KEY_ABC123"
	// A non-empty PrivateKey is required to reach the admin
	// "keys were provided" branch in CreatePeer. The exact value
	// does not matter for this test — only the resulting
	// peer.Identifier, which is derived solely from PublicKey.
	privKey := "TEST_PRIVATE_KEY_XYZ789"

	input := &domain.Peer{
		Identifier:          "should_be_overwritten",
		UserIdentifier:      userId,
		InterfaceIdentifier: domain.InterfaceIdentifier("wg0"),
		Interface: domain.PeerInterfaceConfig{
			KeyPair: domain.KeyPair{PublicKey: pubKey, PrivateKey: privKey},
		},
	}

	// Act
	out, err := m.CreatePeer(ctx, input)

	// Assert
	if err != nil {
		t.Fatalf("CreatePeer returned error: %v", err)
	}

	expectedId := domain.PeerIdentifier(pubKey)
	if out.Identifier != expectedId {
		t.Fatalf("expected Identifier to be set from public key %q, got %q", expectedId, out.Identifier)
	}

	// Ensure the saved peer in DB also has the expected identifier
	if db.savedPeers[expectedId] == nil {
		t.Fatalf("expected peer with identifier %q to be saved in DB", expectedId)
	}
}

func TestCreateDefaultPeer_RespectsInterfaceFlag(t *testing.T) {
	// Arrange
	cfg := &config.Config{}
	cfg.Core.CreateDefaultPeerOnLogin = true

	bus := &mockBus{}
	ctrlMgr := &ControllerManager{
		controllers: map[domain.InterfaceBackend]backendInstance{
			config.LocalBackendName: {Implementation: &mockController{}},
		},
	}

	db := &mockDB{
		iface: &domain.Interface{
			Identifier:        "wg0",
			Type:              domain.InterfaceTypeServer,
			CreateDefaultPeer: false, // Flag is disabled!
		},
	}

	m := Manager{
		cfg: cfg,
		bus: bus,
		db:  db,
		wg:  ctrlMgr,
	}

	userId := domain.UserIdentifier("user@example.com")
	ctx := domain.SetUserInfo(context.Background(), &domain.ContextUserInfo{Id: userId, IsAdmin: true})

	// Act
	err := m.CreateDefaultPeer(ctx, userId)

	// Assert
	if err != nil {
		t.Fatalf("CreateDefaultPeer returned error: %v", err)
	}

	if len(db.savedPeers) != 0 {
		t.Fatalf("expected no peers to be created because interface flag is false, but got %d", len(db.savedPeers))
	}

	// Now enable the flag and try again
	db.iface.CreateDefaultPeer = true
	err = m.CreateDefaultPeer(ctx, userId)

	if err != nil {
		t.Fatalf("CreateDefaultPeer returned error after enabling flag: %v", err)
	}

	if len(db.savedPeers) != 1 {
		t.Fatalf("expected 1 peer to be created because interface flag is true, but got %d", len(db.savedPeers))
	}
}
