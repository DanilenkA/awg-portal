# Changelog

All notable changes to this project are documented in this file.
The format is based on Keep a Changelog 1.1.0, and this project adheres to Semantic Versioning 2.0.0.

## [v2.0.7] - 2026-11-24

### Changed
- Resolve lint findings in the API layer and its tests
- Resolve lint findings in the auth package and the local WireGuard controller
- Resolve lint findings in the MikroTik and pfSense backends
- Resolve lint findings in the domain, application and root packages

### Fixed
- **deps:** Update golang.org/x/text to v0.41.0
- Close real defects found by the linters


[v2.0.7]: https://github.com/DanilenkA/awg-portal/compare/v2.0.6...v2.0.7

## [v2.0.6] - 2026-08-05

### Added
- **auth:** TOTP two-factor authentication (opt-in, database users)
- **ui:** TOTP 2FA login step + settings management + i18n

### Fixed
- **ci:** Reproducible amneziawg-go build + GPG-signed releases
- **db:** Support PrivateString in encstr serializer
- **auth:** Preserve CSRF token across 2FA session transitions
- **ui:** Handle 2FA on the home login form (HomeView)
- **auth:** Allow 2FA enrollment for users without an email
- **ci:** Tag-on-main check needs full history + main/master refs


[v2.0.6]: https://github.com/DanilenkA/awg-portal/compare/v2.0.3...v2.0.6

## [v2.0.3] - 2026-06-22

### Fixed
- **ci:** Normalize buildx binary paths, create platform dir before docker cp
- Guard LoadInterfaces for admin-only to prevent non-admin auto-logout (#55)
- Add admin route guard and update .gitignore (#57)


[v2.0.3]: https://github.com/DanilenkA/awg-portal/compare/vv2.0.0...v2.0.3

## [vv2.0.0] - 2026-06-13

### Added
- V2.0.0-demo — UI/UX redesign, i18n (11 langs), adaptive layout, mobile fixes
- V2.0.0 — AWG peer status fix, traffic polling, traffic drill-down view

### Fixed
- **ci:** Lowercase ghcr.io repository name for docker create
- **ci:** Lowercase ghcr.io repository name for docker create


[vv2.0.0]: https://github.com/DanilenkA/awg-portal/compare/v1.4.2...vv2.0.0

## [v1.4.2] - 2026-06-12

### Fixed
- Баг 1 (ребрендинг AWG Portal) и баг 4 (X25519 в Web Crypto)
- Баг 2 — паника при удалении AWG-интерфейса
- Баги 3 и 5 — PrivateKey/PresharedKey генерация для AWG и PeerStats
- 5 багов — encoding, PrivateKey, PeerStats, rebranding, X25519


[v1.4.2]: https://github.com/DanilenkA/awg-portal/compare/v1.4.1...v1.4.2

## [v1.4.1] - 2026-06-12

### Fixed
- Align install.sh, README and CI bundle packaging with v1.4.0 layout
- Synchronize docs, installer scripts, and add TESTING.md
- **install:** Auto-detect IP + minimal config sample


[v1.4.1]: https://github.com/DanilenkA/awg-portal/compare/v1.4.0...v1.4.1

## [v1.4.0] - 2026-06-11

### Fixed
- Validate awg client config and release docs
- Restore Go fixes and tests from Pre-WebUI audit (lost in rebase)


[v1.4.0]: https://github.com/DanilenkA/awg-portal/compare/v1.3.2...v1.4.0

## [v1.3.2] - 2026-05-31

### Fixed
- Генерация ключей пира при создании через API (PrivateKey/PresharedKey) (#14)
- Update docker-compose image tag to v1.3.1, update comment (#20)
- Nil-pointer crash on Save Config + документация (#26)
- Не создавать TUN-интерфейс для plain WG при awg_mode=auto (#27)


[v1.3.2]: https://github.com/DanilenkA/awg-portal/compare/v1.2.2fix...v1.3.2

## [v1.2.2fix] - 2026-05-29

### Added
- TLS support for web (#301)
- Added peers sorting on views (#302)
- Substitute environment variables in config file (#305)
- Metrics for Prometheus (#309)
- Handle missing config file gracefully with a warning (#331)
- **helm:** Add optional volumeName to persistence configuration #379 (#380)
- Add simple audit ui
- Config by environment variables (#570)
- Introduce "Create Default Peer" flag for interfaces (#513) (#605)
- Allow multiple auth sources per user (#500,#477) (#612)
- Add live traffic stats (#530) (#616)
- Implement LDAP interface-specific provisioning filters (#642)
- **metrics:** Add user label to peer metrics (#647)
- **frontend:** Add confirmation dialog before deleting users, peers, and interfaces (#654)
- Allow encrypting user api token using gorm serializer
- Improve pagination (#662) (#663)
- **i18n:** Add Japanese (ja) translation (#682)
- Sanitize external identity provider user data (#681)
- **docs:** How to troubleshoot admin_group_regex with oidc (#684)
- AWG 2.0 integration with Jipok/wgctrl-go
- **ui:** AWG-PORTAL branding, SVG logo, protocol badge, i18n updates

### Fixed
- LDAP sync interval (#304)
- Autosave wireguard conf files (#303)
- Enhance PrivateString Scan method to support []byte input (#324)
- Fix csrf token handling after login
- Fix session handling (remove IdleTimeout)
- Fix peer audit event
- **helm:** Append prerelease pattern for Helm kubeVersion check (#560)
- Parity of Base64/URL encoding between frontend and backend (#611)
- Typo in local.go (#613)
- Improve import of existing allowed-IPs (#615)
- Improve peer email handling logic (#620)
- Ldap user creation when login in (#631)
- Pin softprops/action-gh-release to 2.4.2 (see https://github.com/softprops/action-gh-release/issues/709)
- Configurable handshake validity interval and improved defaults (#645)
- Prevent interface address clearing during startup (#651)
- "created_at" for users (#656)
- Add AWG fields to API models (v0+v1), rename module to awg-portal
- Add AWG fields to NewInterface() domain→model mapping (v0)
- Fwmark Invert:false, AWG stale socket recovery, protocol selector UI
- Propagate AWG obfuscation params through peer API models
- Direct AWG UAPI peer management (bypass wgctrl-go parse issues)
- Update stale module paths (h44z/wg-portal → DanilenkA/awg-portal) in Makefile and Dockerfile


[v1.2.2fix]: https://github.com/DanilenkA/awg-portal/compare/v1.2.2...v1.2.2fix
