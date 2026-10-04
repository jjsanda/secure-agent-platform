# syntax=docker/dockerfile:1
#
# Builds the control-plane binaries into a distroless, non-root image. Pick which
# binary with the CMD build arg:  CMD=controlplane (default) | CMD=mock-oidc
# Build context is the repository root.

FROM golang:1.25-bookworm AS build
WORKDIR /src/control-plane

# Dependency layer (cached unless go.mod/go.sum change).
COPY control-plane/go.mod control-plane/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Source + build.
COPY control-plane/ ./
ARG CMD=controlplane
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
