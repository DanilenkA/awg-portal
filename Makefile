# awg-portal build system
GOCMD      := go
MODULE     := github.com/DanilenkA/awg-portal
NPMCMD     := npm
BUILDDIR   := dist
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || cat VERSION 2>/dev/null || echo "0.0.0-dev")
COMMIT     := $(shell cd wg-portal && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS    := -w -s -X '$(MODULE)/internal.Version=$(VERSION)'
LDFLAGS    += -X '$(MODULE)/internal.GitCommit=$(COMMIT)'
GOOS       := linux
GOARCH     := amd64

.PHONY: all clean dist frontend binary awg test help version

all: clean awg frontend binary dist

# ---- Version ----
version:
	@echo "VERSION=$(VERSION) COMMIT=$(COMMIT)"

# ---- amneziawg-go ----
awg:
	@echo "[+] Building amneziawg-go for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILDDIR)
	cd amneziawg-go && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOCMD) build \
		-o ../$(BUILDDIR)/amneziawg-go \
		-ldflags "-w -s" \
		-tags netgo \
		.
	@echo "[+] amneziawg-go: $$(ls -lh $(BUILDDIR)/amneziawg-go | awk '{print $$5}')"
	@echo "[+] Static check:" && file $(BUILDDIR)/amneziawg-go

# ---- Frontend ----
frontend:
	@echo "[+] Building frontend..."
	@cd wg-portal/frontend && $(NPMCMD) ci --include=dev 2>&1 | tail -3
	@cd wg-portal/frontend && npx vite build --base=/app/ 2>&1 | tail -3

# ---- awg-portal Binary ----
binary: frontend
	@echo "[+] Building awg-portal $(VERSION) ($(COMMIT)) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILDDIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOCMD) build -C wg-portal \
		-o ../$(BUILDDIR)/awg-portal_x86-64 \
		-ldflags "$(LDFLAGS)" \
		-tags netgo \
		./cmd/wg-portal/
	@echo "[+] awg-portal_x86-64: $$(ls -lh $(BUILDDIR)/awg-portal_x86-64 | awk '{print $$5}')"
	@echo "[+] Static check:" && file $(BUILDDIR)/awg-portal_x86-64

# ---- Dist bundle ----
dist: awg binary
	@echo "[+] Assembling release bundle..."
	@cp deploy/install.sh $(BUILDDIR)/
	@cp wg-portal/config.yml.sample $(BUILDDIR)/
	@cp README.md $(BUILDDIR)/ 2>/dev/null || true
	@echo "$(VERSION)" > $(BUILDDIR)/VERSION
	@echo "[+] Release bundle in $(BUILDDIR)/:"
	@ls -lh $(BUILDDIR)/

# ---- Clean ----
clean:
	@rm -rf wg-portal/frontend/node_modules/
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
	@echo "  all         Clean build → dist bundle (awg-portal + amneziawg-go)"
	@echo "  awg         Build amneziawg-go static binary"
	@echo "  frontend    Build Vue.js frontend"
	@echo "  binary      Build awg-portal_x86-64 static binary"
	@echo "  dist        Assemble release bundle"
	@echo "  test        Run tests"
	@echo "  clean       Remove build artifacts"
	@echo "  version     Show version info"
	@echo ""
	@echo "Override: make GOARCH=arm64 awg binary  (cross-compile)"
