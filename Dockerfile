# Dockerfile References: https://docs.docker.com/engine/reference/builder/
# This dockerfile uses a multi-stage build system to reduce the image footprint.

######
# Build frontend
######
FROM --platform=${BUILDPLATFORM} node:lts-alpine AS frontend
# Set the working directory
WORKDIR /build
# Download dependencies
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
# Set dist output directory
ENV DIST_OUT_DIR="dist"
# Copy the sources to the working directory
COPY frontend .
# Build the frontend
RUN npm run build

######
# Build backend
######
FROM --platform=${BUILDPLATFORM} golang:1.27-alpine AS builder
# Set the working directory
WORKDIR /build
# Download dependencies
COPY go.mod go.sum ./
RUN go mod download
# Copy the sources to the working directory
COPY ./cmd ./cmd
COPY ./internal ./internal
# Copy the frontend build result
COPY --from=frontend /build/dist/ ./internal/app/api/core/frontend-dist/
# Set the build version from arguments
ARG BUILD_VERSION
# Split to cross-platform build
ARG TARGETARCH
# Build the application
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -o /build/dist/wg-portal \
  -ldflags "-w -s -extldflags '-static' -X 'github.com/DanilenkA/awg-portal/internal.Version=${BUILD_VERSION}'" \
  -tags netgo \
  cmd/wg-portal/main.go

######
# Export binaries
######
FROM scratch AS binaries
COPY --from=builder /build/dist/wg-portal /

######
# Build amneziawg-go
######
FROM --platform=${BUILDPLATFORM} golang:1.27-alpine AS amneziawg
ARG TARGETARCH
ARG AMNEZIAWG_COMMIT=1cc94272ca8e9e223a5fe76382f5880f09d3c12d
RUN apk add --no-cache git ca-certificates
WORKDIR /src
# Shallow fetch of a pinned commit. `git clone --depth 1` + `git checkout <sha>`
# is unreliable: the shallow window tracks upstream HEAD, and an old pinned
# commit eventually falls outside it (pathspec error). Fetching the commit by
# SHA directly (smart-HTTP allow-tip/allow-reachable sha1) always resolves.
RUN git init amneziawg && \
    cd amneziawg && \
    git remote add origin https://github.com/amnezia-vpn/amneziawg-go.git && \
    git fetch --depth 1 origin "${AMNEZIAWG_COMMIT}" && \
    git checkout FETCH_HEAD && \
    CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -ldflags "-w -s" -o /out/amneziawg-go .

######
# Export amneziawg-go binary (isolated from Go module cache / .git)
######
FROM scratch AS amneziawg-bin
COPY --from=amneziawg /out/amneziawg-go /amneziawg-go

######
# Final image
######
FROM alpine:3.24
# Install OS-level dependencies
RUN apk add --no-cache bash curl iptables nftables openresolv wireguard-tools tzdata
# Setup timezone
ENV TZ=UTC
# Copy binaries
COPY --from=builder /build/dist/wg-portal /app/wg-portal
COPY --from=amneziawg-bin /amneziawg-go /usr/local/bin/amneziawg-go
# Set the Current Working Directory inside the container
WORKDIR /app
# Expose default ports for metrics, web and wireguard
EXPOSE 8787/tcp
EXPOSE 8888/tcp
EXPOSE 51820/udp
# the database and config file can be mounted from the host
VOLUME [ "/app/data", "/app/config" ]
# Command to run the executable
ENTRYPOINT [ "/app/wg-portal" ]
