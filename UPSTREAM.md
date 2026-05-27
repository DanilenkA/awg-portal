# Upstream update guide

## wg-portal base

The fork is based on [h44z/wg-portal](https://github.com/h44z/wg-portal),
with AWG patches applied on top of `feature/awg-backend`.

```bash
# Add upstream remote (once)
git remote add upstream https://github.com/h44z/wg-portal

# Fetch and merge latest upstream changes
git fetch upstream
git merge upstream/master

# Resolve conflicts, then:
cd wg-portal
go mod tidy
go build -tags netgo ./...
go test ./internal/... -short -count=1
```

**Key integration points** (check these after merge):
- `internal/adapters/wgcontroller/local.go` — AWG process lifecycle
- `internal/lowlevel/awg.go` — AWG parameter model
- `internal/config/backend.go` — `awg_mode` config option
- `cmd/wg-portal/main.go` — `defer StopAllAWGProcesses()`
- `go.mod` — uses `github.com/Jipok/wgctrl-go` (not upstream wgctrl)

## amneziawg-go binary

amneziawg-go is an external binary (not a Go dependency).
Build separately and install to `/usr/local/bin/`.

```bash
# From amneziawg-go submodule
cd amneziawg-go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -o /usr/local/bin/amneziawg-go \
  -tags netgo \
  .

# Verify
file /usr/local/bin/amneziawg-go
# ELF 64-bit, statically linked

# Update submodule
git submodule update --remote amneziawg-go
```

## Reproducible build

```bash
make clean
make dist
file dist/wg-portal-amd64
# statically linked, stripped
ldd dist/wg-portal-amd64
# not a dynamic executable
```

## Version history

| Version | Date | Notes |
|---------|------|-------|
| v1.0.0 | 2026-05-27 | AWG 2.0 integration, Jipok/wgctrl-go, multi-interface |
| v0.1.0-awg | 2026-05-26 | Experimental AWG backend |
| v0.1.0 | 2026-05-26 | Initial fork |
