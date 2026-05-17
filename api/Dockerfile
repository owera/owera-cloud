# Multi-stage build for cmd/apiserver.
#
# Stage 1 (builder): pulls the pure-Go toolchain, downloads modules with
# a cached layer, then statically compiles the apiserver binary. The
# project uses modernc.org/sqlite (pure Go) so CGO_ENABLED=0 produces a
# fully-static binary that runs on the scratch/distroless runtime stage.
#
# Stage 2 (runtime): distroless static-nonroot image — no shell, no
# package manager, no setuid binaries. UID 65532 (nonroot). Port 8080
# matches infra/api.fly.toml + the /healthz contract.
#
# Build context: this Dockerfile expects to be built with the api/
# directory as the build context. The companion fly.toml lives at
# infra/api.fly.toml; deploy with:
#
#     cd api && fly deploy --config ../infra/api.fly.toml
#
# The cd makes the cwd the build context (api/), and --config points
# fly at the toml in infra/. fly.toml's dockerfile reference resolves
# back to api/Dockerfile via its `../api/Dockerfile` relative path.

ARG GO_VERSION=1.23
ARG ALPINE_VERSION=3.20

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder
WORKDIR /src

# Step 1: cache module downloads in their own layer. Re-running the
# build after a source-only change skips this layer.
COPY go.mod go.sum ./
RUN go mod download

# Step 2: copy the source tree. The .dockerignore (sibling file) keeps
# test/build artefacts out of the context.
COPY . .

# Step 3: build the apiserver binary. -trimpath + -ldflags strip
# absolute paths and DWARF info; the resulting binary is ~12 MB.
# Pin GOOS/GOARCH so the build is reproducible regardless of the host
# (Fly remote-only builders run linux/amd64 by default; M-series
# operator Macs build linux/arm64 by default — pin to linux/amd64 so
# Fly's gru region runs the same artefact).
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}
RUN go build \
        -trimpath \
        -ldflags "-s -w" \
        -o /out/apiserver \
        ./cmd/apiserver

# --- runtime stage ---

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /

COPY --from=builder /out/apiserver /apiserver

# SQLite database lives at /data/owera-cloud.db so a Fly volume can be
# mounted at /data for persistence across deploys. Without a volume the
# DB still works but is wiped on each fly deploy / machine restart.
ENV OWERA_DB_PATH=/data/owera-cloud.db

EXPOSE 8080

# distroless nonroot UID. SQLite needs to write to /data, so the
# operator must `chown 65532:65532 /data` once after mounting the
# volume (or fly will run as nonroot and SQLite will fail on first
# write). Documented in infra/README.md.
USER nonroot:nonroot

# The apiserver binary reads --addr / --db flags. Defaults match
# fly.toml's PORT env (:8080) and a writable /data path. Override at
# fly-deploy time via `flyctl machine update --metadata` or by editing
# fly.toml's [processes] section.
ENTRYPOINT ["/apiserver"]
CMD ["--addr=:8080", "--db=/data/owera-cloud.db"]
