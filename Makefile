# API Regression Guard — common development and release tasks.
# Run `make help` for the list of targets.

APP      := api-regression-guard
MODULE   := api-test-kits
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 0.1.0-dev)
GO       ?= go
WAILS    ?= $(shell command -v wails 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/wails)
HOST_OS  := $(shell $(GO) env GOHOSTOS)
HOST_ARCH := $(shell $(GO) env GOHOSTARCH)

# Linux needs the webkit2_41 tag when WebKitGTK 4.1 is installed (Ubuntu 24.04+).
ifeq ($(HOST_OS),linux)
  TAGS ?= $(shell pkg-config --exists webkit2gtk-4.1 2>/dev/null && echo webkit2_41)
endif
TAGS_FLAG := $(if $(TAGS),-tags $(TAGS),)
LDFLAGS   := -s -w -X $(MODULE)/internal/service.Version=$(VERSION)

ifeq ($(HOST_OS),windows)
  BIN := build/bin/$(APP).exe
else
  BIN := build/bin/$(APP)
endif

.DEFAULT_GOAL := help
.PHONY: help deps tools doctor dev run build test test-go test-frontend lint fmt frontend \
        build-linux build-debian build-windows build-windows-amd64 build-windows-arm64 build-darwin \
        release release-windows-installer icons clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---- setup ----

deps: ## Install frontend dependencies and download Go modules
	cd frontend && npm ci --no-audit --no-fund
	$(GO) mod download

tools: ## Install the Wails CLI
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2

doctor: ## Check that the system has everything Wails needs
	$(WAILS) doctor

# ---- develop and run ----

dev: ## Run the app with hot reload (frontend + Go)
	$(WAILS) dev $(TAGS_FLAG)

build: frontend ## Build the desktop app for this machine (build/bin/)
	$(WAILS) build -s -m -trimpath $(TAGS_FLAG) -ldflags "$(LDFLAGS)"

run: build ## Build, then start the desktop app
	./$(BIN)

frontend: frontend/node_modules ## Build the frontend into frontend/dist
	cd frontend && npm run build

frontend/node_modules: frontend/package-lock.json
	cd frontend && npm ci --no-audit --no-fund
	@touch $@

# ---- quality ----

test: test-frontend test-go ## Run all tests

test-frontend: frontend/node_modules
	cd frontend && npm test

test-go: frontend ## Go tests (main.go embeds frontend/dist, so it is built first)
	$(GO) test ./...

lint: frontend/node_modules ## Frontend formatting/line-length checks and go vet
	cd frontend && npm run lint
	$(GO) vet ./...

fmt: frontend/node_modules ## Format Go and frontend code
	gofmt -w $(shell git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './frontend/*')
	cd frontend && npm run format

# ---- per-platform builds (binaries in build/bin/, no packaging) ----

build-linux: ## Linux binary for this machine's architecture
	scripts/build.sh --skip-tests --no-package -v $(VERSION) linux

build-debian: ## .deb for Debian 12+/Ubuntu 22.04+, built in a Debian 12 container (needs Docker)
	scripts/build-debian.sh -v $(VERSION)

build-windows: ## Windows amd64 + arm64 .exe (cross-compiles from Linux)
	scripts/build.sh --skip-tests --no-package -v $(VERSION) windows

build-windows-amd64: ## Windows amd64 .exe
	scripts/build.sh --skip-tests --no-package -v $(VERSION) windows/amd64

build-windows-arm64: ## Windows arm64 .exe
	scripts/build.sh --skip-tests --no-package -v $(VERSION) windows/arm64

build-darwin: ## macOS universal .app (macOS host only)
	scripts/build.sh --skip-tests --no-package -v $(VERSION) darwin

# ---- release ----

release: ## Test, then build and package every platform this host can build (release/)
	scripts/build.sh -v $(VERSION) all

release-windows-installer: ## Windows builds plus NSIS installers (needs makensis)
	scripts/build.sh -v $(VERSION) --nsis windows

icons: ## Regenerate PNG/ICO icons from build/appicon.svg
	python3 scripts/render-icon.py

clean: ## Remove build outputs
	rm -rf build/bin release frontend/dist
