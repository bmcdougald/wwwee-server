# syntax=docker/dockerfile:1.7

# --------------------------------------------------------------------------
# Build stage: compile the application with the validated Go Cryptographic
# Module. Nothing from this stage (compiler, shell, package manager) ships
# in the final image.
# --------------------------------------------------------------------------
ARG GO_VERSION=1.26.8
FROM golang:${GO_VERSION}-alpine AS build

ARG GOFIPS140_VERSION=v1.0.0
WORKDIR /src

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=mod \
    GOFIPS140=${GOFIPS140_VERSION}

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go vet ./... \
    && go build -trimpath -ldflags="-s -w" -o /out/wwwee-server ./cmd/server \
    && go version -m /out/wwwee-server | grep -q "GOFIPS140.*${GOFIPS140_VERSION}"

# --------------------------------------------------------------------------
# Final stage: distroless, non-root, no shell/package manager/compiler.
# The application contains the validated Go cryptographic module in its
# binary; OpenSSL is intentionally not required by this architecture.
# --------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS final

LABEL org.opencontainers.image.title="wwwee-server" \
      org.opencontainers.image.description="Minimal hardened static web server for a department intranet" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.security.fips140-3="Go Cryptographic Module v1.0.0 (CMVP #5247)"

WORKDIR /

COPY --from=build --chown=nonroot:nonroot /out/wwwee-server /wwwee-server

ENV WWWEE_ADDR=:8080 \
    WWWEE_CONTENT_DIR=/srv/content \
    GODEBUG=fips140=on

# Non-root user provided by the distroless "nonroot" variant (uid/gid 65532).
USER nonroot:nonroot

EXPOSE 8080

# Site content is supplied by externally managed persistent storage and should
# be mounted read-only. The deployment environment owns the storage location.
# No Docker-managed VOLUME is declared here; the content mount is intentionally
# supplied by the runtime/deployment configuration.

# The image has no shell, so the binary performs its own health probe.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/wwwee-server", "-healthcheck"]

ENTRYPOINT ["/wwwee-server"]
