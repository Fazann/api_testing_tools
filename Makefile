# API test kits — common development and release tasks.
# Run `make help` for the list of targets.

APP      := api-test-kits
MODULE   := api-test-kits
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 0.1.0-dev)
GO       ?= go
WAILS    ?= $(shell command -v wails 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/wails)
# Installed apps poll this manifest and update themselves in place. Upload
# release/latest.json next to the release files (the GitHub "latest" release
# serves it at this URL; the repository must be public, or point this at any
# public HTTPS host). Set UPDATE_URL= to build without in-app updates.
REPO        ?= Fazann/api_testing_tools
UPDATE_URL  ?= https://github.com/$(REPO)/releases/latest/download/latest.json
RELEASE_PAGE_URL ?= https://github.com/$(REPO)/releases/tag/v{version}
# NOTES (release notes, may span lines) reaches scripts/bump-version.sh through the environment.
export UPDATE_URL RELEASE_PAGE_URL NOTES
HOST_OS  := $(shell $(GO) env GOHOSTOS)
HOST_ARCH := $(shell $(GO) env GOHOSTARCH)

# Linux needs the webkit2_41 tag when WebKitGTK 4.1 is installed (Ubuntu 24.04+).
ifeq ($(HOST_OS),linux)
  TAGS ?= $(shell pkg-config --exists webkit2gtk-4.1 2>/dev/null && echo webkit2_41)
endif
TAGS_FLAG := $(if $(TAGS),-tags $(TAGS),)
LDFLAGS   := -s -w -X $(MODULE)/internal/service.Version=$(VERSION)
DESKTOP_LDFLAGS := $(LDFLAGS) -X $(MODULE)/internal/updater.FeedURL=$(UPDATE_URL)

ifeq ($(HOST_OS),windows)
  BIN := build/bin/$(APP).exe
else
  BIN := build/bin/$(APP)
endif

.DEFAULT_GOAL := help
.PHONY: help deps tools doctor dev run build test test-go test-frontend lint fmt frontend check hooks \
        build-linux build-debian build-windows build-windows-amd64 build-windows-arm64 build-darwin \
        release release-windows-installer icons clean build-cli release-cli \
        version bump-patch bump-minor bump-major manifest release-all release-patch release-minor release-major

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
	$(WAILS) build -s -m -trimpath $(TAGS_FLAG) -ldflags "$(DESKTOP_LDFLAGS)"

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

check: frontend/node_modules ## Regression check: lint, typecheck, gofmt, go vet and all tests
	scripts/check.sh

hooks: ## Install the git pre-commit hook that runs the regression check on staged changes
	git config core.hooksPath scripts/git-hooks
	@echo "Pre-commit regression check installed (skip once with: git commit --no-verify)"

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

build-cli: ## Build the headless CLI for this machine (build/bin/api-test-kits-cli, pure Go)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o build/bin/api-test-kits-cli$(if $(filter windows,$(HOST_OS)),.exe,) ./cmd/api-test-kits-cli

release-cli: ## Cross-compile and package the CLI for Linux, Windows and macOS (release/)
	scripts/build-cli.sh -v $(VERSION)

release: ## New release: bump the patch version (every run), commit, tag, test, build all packages + latest.json
	scripts/bump-version.sh patch
	$(MAKE) release-all

release-windows-installer: ## Windows builds plus NSIS installers (needs makensis)
	scripts/build.sh -v $(VERSION) --nsis windows

# ---- versioned releases with in-app updates ----
# Patch release:  make release-patch NOTES="Fix ..."   then   git push --follow-tags
# Pushing the tag publishes release/* to GitHub (.github/workflows/release.yml);
# installed apps then offer "Update and restart" — no uninstall needed.

version: ## Print the version the next build will carry
	@echo $(VERSION)

bump-patch: ## Bump X.Y.Z -> X.Y.Z+1, commit and tag (fixes, small changes). NOTES="..." sets release notes
	scripts/bump-version.sh patch

bump-minor: ## Bump X.Y.Z -> X.Y+1.0, commit and tag (new features)
	scripts/bump-version.sh minor

bump-major: ## Bump X.Y.Z -> X+1.0.0, commit and tag (breaking changes)
	scripts/bump-version.sh major

manifest: ## Write release/latest.json (the update manifest) for the current version
	scripts/make-manifest.sh -v $(VERSION)

release-all: ## Build every update asset this host can (.deb needs Docker, installer needs makensis) + latest.json
	scripts/build.sh -v $(VERSION) $(if $(shell command -v makensis),--nsis,) all
	@if command -v $${DOCKER:-docker} >/dev/null; then scripts/build-debian.sh -v $(VERSION); \
	  else echo "==> Docker not found: skipping the .deb (.deb installs will point to the download page)"; fi
	scripts/make-manifest.sh -v $(VERSION)

release-patch: ## Bump the patch version, tag it, build everything and write latest.json
	scripts/bump-version.sh patch
	$(MAKE) release-all

release-minor: ## Bump the minor version, tag it, build everything and write latest.json
	scripts/bump-version.sh minor
	$(MAKE) release-all

release-major: ## Bump the major version, tag it, build everything and write latest.json
	scripts/bump-version.sh major
	$(MAKE) release-all

icons: ## Regenerate PNG/ICO icons from build/appicon.svg
	python3 scripts/render-icon.py

clean: ## Remove build outputs
	rm -rf build/bin release frontend/dist
