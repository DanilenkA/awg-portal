package model

type Error struct {
	Code    int    `json:"Code"`
	Message string `json:"Message"`
}

type Settings struct {
	MailLinkOnly              bool                   `json:"MailLinkOnly"`
	PersistentConfigSupported bool                   `json:"PersistentConfigSupported"`
	SelfProvisioning          bool                   `json:"SelfProvisioning"`
	ApiAdminOnly              bool                   `json:"ApiAdminOnly"`
	WebAuthnEnabled           bool                   `json:"WebAuthnEnabled"`
	MinPasswordLength         int                    `json:"MinPasswordLength"`
	AvailableBackends         []SettingsBackendNames `json:"AvailableBackends"`
	LoginFormVisible          bool                   `json:"LoginFormVisible"`
	CreateDefaultPeer         bool                   `json:"CreateDefaultPeer"`
	// AWGAvailable is true when the "amneziawg-go" binary is reachable via
	// the current PATH. The frontend uses it to surface a soft warning when
	// the operator tries to enable AmneziaWG obfuscation on a host that
	// does not have the binary installed — saving a 500 error round-trip.
	AWGAvailable              bool                   `json:"AWGAvailable"`
}

type SettingsBackendNames struct {
	Id   string `json:"Id"`
	Name string `json:"Name"`
}
