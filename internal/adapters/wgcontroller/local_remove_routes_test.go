package wgcontroller

import (
	"errors"
	"testing"
	"time"

	"github.com/Jipok/wgctrl-go/wgtypes"
	"github.com/vishvananda/netlink"

	"github.com/DanilenkA/awg-portal/internal/config"
	"github.com/DanilenkA/awg-portal/internal/domain"
	"github.com/DanilenkA/awg-portal/internal/lowlevel"
)

// fakeNetlink возвращает ошибку для LinkByName — имитирует ситуацию, когда
// физический netlink-линк уже снесён (как при удалении AWG-интерфейса).
type fakeNetlink struct {
	lowlevel.NetlinkManager
	linkByNameErr error
}

func (f *fakeNetlink) LinkByName(_ string) (netlink.Link, error) {
	return nil, f.linkByNameErr
}

// failingWgCtrlRepo — wgctrl, который всегда возвращает ошибку для Device
// (имитация того, что wg-устройство тоже уже снесено).
type failingWgCtrlRepo struct{}

func (failingWgCtrlRepo) Close() error { return nil }
func (failingWgCtrlRepo) Devices() ([]*wgtypes.Device, error) {
	return nil, nil
}
func (failingWgCtrlRepo) Device(_ string) (*wgtypes.Device, error) {
	return nil, errors.New("device not found")
}
func (failingWgCtrlRepo) ConfigureDevice(_ string, _ wgtypes.Config) error {
	return nil
}

// TestRemoveRoutes_LinkAlreadyGoneDoesNotPanic — регрессионный тест бага 2:
// при удалении AWG-интерфейса физический netlink-линк может быть уже снесён
// к моменту route cleanup. До фикса код безусловно дёргал link.Attrs().Index
// и падал в nil pointer dereference, из-за чего весь сервис падал в panic
// при удалении AWG-интерфейса. Этот тест проверяет, что RemoveRoutes
// возвращает nil, а не паникует, когда LinkByName возвращает ошибку.
func TestRemoveRoutes_LinkAlreadyGoneDoesNotPanic(t *testing.T) {
	// Имитируем ситуацию "линк уже удалён": LinkByName возвращает ошибку,
	// похожую на netlink.LinkNotFoundError.
	notFound := &netlink.LinkNotFoundError{}
	fakeNl := &fakeNetlink{linkByNameErr: notFound}

	c := LocalController{
		cfg: &config.Config{
			Advanced: struct {
				LogLevel                 string        `yaml:"log_level"`
				LogPretty                bool          `yaml:"log_pretty"`
				LogJson                  bool          `yaml:"log_json"`
				StartListenPort          int           `yaml:"start_listen_port"`
				StartCidrV4              string        `yaml:"start_cidr_v4"`
				StartCidrV6              string        `yaml:"start_cidr_v6"`
				UseIpV6                  bool          `yaml:"use_ip_v6"`
				ConfigStoragePath        string        `yaml:"config_storage_path"`
				ExpiryCheckInterval      time.Duration `yaml:"expiry_check_interval"`
				RulePrioOffset           int           `yaml:"rule_prio_offset"`
				RouteTableOffset         int           `yaml:"route_table_offset"`
				ApiAdminOnly             bool          `yaml:"api_admin_only"`
				LimitAdditionalUserPeers int           `yaml:"limit_additional_user_peers"`
			}{
				RouteTableOffset: 20000,
			},
		},
		wg: failingWgCtrlRepo{},
		nl: fakeNl,
	}

	iface := domain.Interface{
		BaseModel:    domain.BaseModel{},
		Identifier:   "awg-deleted",
		KeyPair:      domain.KeyPair{},
		Backend:      config.LocalBackendName,
		Addresses:    nil,
		FirewallMark: 0,
	}
	info := domain.RoutingTableInfo{
		Interface: iface,
		IsDeleted: true,
	}

	// Без фикса — тут сервис бы упал в panic. С фиксом — должно вернуться nil.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RemoveRoutes упал в panic на отсутствующем линке: %v", r)
		}
	}()

	if err := c.RemoveRoutes(t.Context(), info); err != nil {
		t.Fatalf("RemoveRoutes вернул ошибку при отсутствующем линке (ожидался тихий выход): %v", err)
	}
}