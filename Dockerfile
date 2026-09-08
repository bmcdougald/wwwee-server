# syntax=docker/dockerfile:1.7

# --------------------------------------------------------------------------
# Build stage: compiles a static, dependency-free binary. Nothing from this
# stage (compiler, shell, package manager) ships in the final image.
# --------------------------------------------------------------------------
ARG GO_VERSION=1.23
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=mod

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go vet ./... \
    && go build -trimpath -ldflags="-s -w" -o /out/wwwee-server ./cmd/server

# --------------------------------------------------------------------------
# Final stage: distroless, non-root, no shell/package manager/compiler -
# satisfies the Container Platform SRG minimization requirements.
# --------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS final

LABEL org.opencontainers.image.title="wwwee-server" \
      org.opencontainers.image.description="Minimal hardened static web server for a department intranet" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /

COPY --from=build --chown=nonroot:nonroot /out/wwwee-server /wwwee-server
# Default content baked in only as a fallback; mount over /srv/content in
# production to serve real site content without rebuilding the image.
COPY --chown=nonroot:nonroot content/ /srv/content/

ENV WWWEE_ADDR=:8080 \
    WWWEE_CONTENT_DIR=/srv/content

# Non-root user provided by the distroless "nonroot" variant (uid/gid 65532).
USER nonroot:nonroot

EXPOSE 8080

# Web content mount point. Also mount TLS material read-only at a path of
# your choosing and point WWWEE_TLS_CERT_FILE / WWWEE_TLS_KEY_FILE at it.
VOLUME ["/srv/content"]

# The image has no shell, so the binary performs its own health probe.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/wwwee-server", "-healthcheck"]

ENTRYPOINT ["/wwwee-server"]
