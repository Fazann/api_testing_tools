# Build environment for the Debian/Ubuntu package (used by scripts/build-debian.sh).
# Debian 12 has glibc 2.36 and WebKitGTK 4.1, so binaries built here run on
# Debian 12+, Ubuntu 22.04+ and their derivatives (Mint, Pop!_OS, elementary, ...).
FROM debian:bookworm

ARG GO_VERSION=1.26.8
ARG NODE_VERSION=22.12.0
ARG WAILS_VERSION=v2.10.2

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      build-essential pkg-config ca-certificates curl xz-utils git file \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
 && rm -rf /var/lib/apt/lists/*

RUN arch="$(dpkg --print-architecture)" \
 && case "$arch" in amd64) node_arch=x64 ;; arm64) node_arch=arm64 ;; *) echo "unsupported arch $arch" >&2; exit 1 ;; esac \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -C /usr/local -xz \
 && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" \
      | tar -C /usr/local --strip-components=1 -xJ

ENV PATH=/usr/local/go/bin:$PATH
RUN GOBIN=/usr/local/bin GOPATH=/tmp/gopath go install github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION} \
 && rm -rf /tmp/gopath /root/.cache

# rsvg-convert renders the app icon sizes for the package.
RUN apt-get update \
 && apt-get install -y --no-install-recommends librsvg2-bin \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
