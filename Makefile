# awg-portal build system
GOCMD   := go
MODULE  := github.com/h44z/wg-portal
NPMCMD  := npm
BUILDDIR:= dist
VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0-dev")
COMMIT  := $(shell cd wg-portal && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -w -s -extldflags "-static" -X '$(MODULE)/internal.Version=$(VERSION)-$(COMMIT)'

.PHONY: all clean dist frontend binary test help

all: clean frontend binary dist

# ---- Frontend ----
frontend:
	@echo "[+] Building frontend..."
	@cd wg-portal/frontend && $(NPMCMD) install --include=dev 2>&1 | tail -3
	@cd wg-portal/frontend && node node_modules/vite/bin/vite.js build --base=/app/ 2>&1 | tail -3

# ---- Binary ----
binary:
	@echo "[+] Building wg-portal (version: $(VERSION)-$(COMMIT))..."
	@mkdir -p $(BUILDDIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build \
		-o $(BUILDDIR)/wg-portal-amd64 \
		-ldflags "$(LDFLAGS)" \
		-tags netgo \
		./wg-portal/cmd/wg-portal/
	@echo "[+] Binary: $$(ls -lh $(BUILDDIR)/wg-portal-amd64 | awk '{print $$5}')"

# ---- ARM cross-build (requires cross-compiler) ----
binary-arm64:
	@mkdir -p $(BUILDDIR)
	CGO_ENABLED=0 CC=aarch64-linux-gnu-gcc GOOS=linux GOARCH=arm64 $(GOCMD) build \
		-o $(BUILDDIR)/wg-portal-arm64 \
		-ldflags "$(LDFLAGS)" \
		-tags netgo \
		./wg-portal/cmd/wg-portal/

binary-arm:
	@mkdir -p $(BUILDDIR)
	CGO_ENABLED=0 CC=arm-linux-gnueabi-gcc GOOS=linux GOARCH=arm GOARM=7 $(GOCMD) build \
		-o $(BUILDDIR)/wg-portal-arm \
		-ldflags "$(LDFLAGS)" \
		-tags netgo \
		./wg-portal/cmd/wg-portal/

# ---- Dist bundle ----
dist: frontend binary
	@echo "[+] Assembling release bundle..."
	@cp wg-portal/scripts/wg-portal.service $(BUILDDIR)/
	@cp deploy/systemd/amneziawg@.service $(BUILDDIR)/
	@cp deploy/systemd/override-wg-portal.conf $(BUILDDIR)/
	@cp README.md $(BUILDDIR)/
	@cp VERSION $(BUILDDIR)/ 2>/dev/null || true
	@cp deploy/install.sh $(BUILDDIR)/ 2>/dev/null || true
	@echo "[+] Release bundle in $(BUILDDIR)/:"
	@ls -la $(BUILDDIR)/

# ---- Clean ----
clean:
	@rm -rf wg-portal/internal/app/api/core/frontend-dist/
	@rm -rf $(BUILDDIR)
	@echo "[+] Cleaned."

# ---- Test ----
test:
	cd wg-portal && $(GOCMD) test ./internal/... -short -count=1

# ---- Help ----
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "  all             Build everything (dist bundle)"
	@echo "  frontend        Build Vue.js frontend"
	@echo "  binary          Build amd64 binary"
	@echo "  binary-arm64    Build arm64 binary (cross)"
	@echo "  binary-arm      Build arm32 binary (cross)"
	@echo "  dist            Assemble release bundle"
	@echo "  test            Run tests"
	@echo "  clean           Clean"
