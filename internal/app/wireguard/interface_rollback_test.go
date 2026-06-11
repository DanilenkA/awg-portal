package wireguard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
	"github.com/DanilenkA/awg-portal/internal/lowlevel"
)

func TestSaveInterface_DBFailureAfterModeSwitchRestoresPreviousPhysicalState(t *testing.T) {
	keyPair, err := domain.NewFreshKeypair()
	if err != nil {
		t.Fatalf("failed to create keypair: %v", err)
	}

	oldIface := &domain.Interface{
		Identifier: "wg0",
		KeyPair:    keyPair,
		ListenPort: 51820,
		Type:       domain.InterfaceTypeServer,
		Backend:    config.LocalBackendName,
	}
	newIface := *oldIface
	newIface.AWGEnabled = true
	setInterfaceAWGParams(&newIface)

	peer := domain.Peer{Identifier: "peer1"}
	dbErr := errors.New("db is down")
	db := &rollbackDB{
		mockDB:  mockDB{iface: oldIface},
		saveErr: dbErr,
		peers:   []domain.Peer{peer},
	}
	controller := &rollbackController{}
	manager := Manager{
		cfg: &config.Config{},
		bus: &mockBus{},
		db:  db,
		wg: &ControllerManager{
			controllers: map[domain.InterfaceBackend]backendInstance{
				config.LocalBackendName: {Implementation: controller},
			},
		},
	}

	_, err = manager.saveInterface(context.Background(), &newIface)
	if err == nil || !strings.Contains(err.Error(), "failed to save interface") {
		t.Fatalf("expected DB save failure, got %v", err)
	}

	if len(controller.savedAWG) != 2 {
		t.Fatalf("expected desired apply and previous restore, got savedAWG=%v", controller.savedAWG)
	}
	if !controller.savedAWG[0] {
		t.Fatalf("expected first physical save to apply AWG state, got savedAWG=%v", controller.savedAWG)
	}
	if controller.savedAWG[1] {
		t.Fatalf("expected rollback to restore plain WG state, got savedAWG=%v", controller.savedAWG)
	}
	if len(controller.deleted) != 2 {
		t.Fatalf("expected mode-switch delete and rollback delete, got %v", controller.deleted)
	}
	if len(controller.savedPeers) != 1 || controller.savedPeers[0] != peer.Identifier {
		t.Fatalf("expected rollback to restore peer %q, got %v", peer.Identifier, controller.savedPeers)
	}
}

func setInterfaceAWGParams(iface *domain.Interface) {
	params := lowlevel.AWGParams{
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
	}
	iface.AWGJc = params.Jc
	iface.AWGJmin = params.Jmin
	iface.AWGJmax = params.Jmax
	iface.AWGS1 = params.S1
	iface.AWGS2 = params.S2
	iface.AWGS3 = params.S3
	iface.AWGS4 = params.S4
	iface.AWGH1 = params.H1
	iface.AWGH2 = params.H2
	iface.AWGH3 = params.H3
	iface.AWGH4 = params.H4
}

type rollbackDB struct {
	mockDB
	saveErr error
	peers   []domain.Peer
}

func (r *rollbackDB) SaveInterface(
	_ context.Context,
	_ domain.InterfaceIdentifier,
	_ func(in *domain.Interface) (*domain.Interface, error),
) error {
	return r.saveErr
}

func (r *rollbackDB) GetInterfacePeers(_ context.Context, _ domain.InterfaceIdentifier) ([]domain.Peer, error) {
	return r.peers, nil
}

type rollbackController struct {
	savedAWG   []bool
	deleted    []domain.InterfaceIdentifier
	savedPeers []domain.PeerIdentifier
}

func (r *rollbackController) GetId() domain.InterfaceBackend { return config.LocalBackendName }

func (r *rollbackController) GetInterfaces(_ context.Context) ([]domain.PhysicalInterface, error) {
	return nil, nil
}

func (r *rollbackController) GetInterface(_ context.Context, id domain.InterfaceIdentifier) (*domain.PhysicalInterface, error) {
	return &domain.PhysicalInterface{Identifier: id}, nil
}

func (r *rollbackController) GetPeers(_ context.Context, _ domain.InterfaceIdentifier) ([]domain.PhysicalPeer, error) {
	return nil, nil
}

func (r *rollbackController) SaveInterface(
	_ context.Context,
	_ domain.InterfaceIdentifier,
	updateFunc func(pi *domain.PhysicalInterface) (*domain.PhysicalInterface, error),
) error {
	pi, err := updateFunc(&domain.PhysicalInterface{})
	if err != nil {
		return err
	}
	r.savedAWG = append(r.savedAWG, pi.EmitAWG())
	return nil
}

func (r *rollbackController) DeleteInterface(_ context.Context, id domain.InterfaceIdentifier) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *rollbackController) SavePeer(
	_ context.Context,
	_ domain.InterfaceIdentifier,
	id domain.PeerIdentifier,
	updateFunc func(pp *domain.PhysicalPeer) (*domain.PhysicalPeer, error),
) error {
	if _, err := updateFunc(&domain.PhysicalPeer{}); err != nil {
		return err
	}
	r.savedPeers = append(r.savedPeers, id)
	return nil
}

func (r *rollbackController) DeletePeer(_ context.Context, _ domain.InterfaceIdentifier, _ domain.PeerIdentifier) error {
	return nil
}

func (r *rollbackController) PingAddresses(_ context.Context, _ string) (*domain.PingerResult, error) {
	return nil, nil
}
