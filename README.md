# wwwee-server

A minimal, hardened static web server for hosting a small department
intranet site. It is designed to run as a container — standalone with
Docker/Podman or orchestrated with Kubernetes — and to be as small and
boring as possible.

## Design goals

- **Minimal dependencies.** The server is written in Go using only the
  standard library (`net/http`, `log/slog`, etc.) — zero third-party
  packages, so there is nothing to patch when a dependency has a CVE.
- **Minimal runtime image.** Multi-stage build; the final image is
  [`gcr.io/distroless/static-debian12:nonroot`](https://github.com/GoogleContainerTools/distroless) —
  no shell, no package manager, no compiler, no unnecessary binaries.
- **Hardened by default.** Non-root user, read-only root filesystem
  compatible, all Linux capabilities droppable, restrictive security
  headers, method allow-listing, path-traversal and dotfile protection,
  request timeouts, and graceful shutdown.
- **Kubernetes- and standalone-friendly.** Works as a bare `docker run`
  container or behind a Kubernetes `Deployment`, with `/healthz` (liveness)
  and `/readyz` (readiness) endpoints and example manifests in [k8s/](k8s).

## Hardening approach (Container Platform SRG / Kubernetes STIG)

DISA does not publish a container-specific STIG; container hardening is
assessed against the **Container Platform SRG**, the **Kubernetes STIG**,
and the **GPOS SRG**, with most host/kernel-level controls (auditing,
ASLR, firewalling, filesystem encryption, time sync) inherited from the
underlying host and orchestrator rather than owned by the image. This
project focuses on the controls the _image_ is actually responsible for:

| Control                                          | How it's addressed                                                                                                    |
| ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| Minimization (no shell/package manager/compiler) | `distroless/static` final stage; build tools only exist in the discarded build stage                                  |
| Non-root user                                    | Runs as the distroless `nonroot` user (uid/gid `65532`)                                                               |
| Read-only root filesystem                        | Server performs no writes; verified with `--read-only`                                                                |
| Capability dropping                              | Verified with `--cap-drop=ALL`; no capabilities are required                                                          |
| No privilege escalation                          | `--security-opt no-new-privileges`                                                                                    |
| Process isolation / least privilege              | Static site only serves `GET`/`HEAD`; all other methods return `405`                                                  |
| TLS 1.2+                                         | `tls.Config{MinVersion: tls.VersionTLS12}` when `WWWEE_TLS_CERT_FILE`/`WWWEE_TLS_KEY_FILE` are set                    |
| Vulnerability surface reduction                  | Zero third-party Go modules; small stdlib-only binary                                                                 |
| Health/liveness for orchestration                | `/healthz` (liveness) and `/readyz` (readiness)                                                                       |
| Auditing                                         | Structured JSON access logs to stdout for collection by the container runtime/host, not written to files in-container |
| Validated against GPOS SRG (image-level)         | [scripts/openscap-scan.sh](scripts/openscap-scan.sh) runs an OpenSCAP scan and gates on unaccepted findings           |

See [k8s/deployment.yaml](k8s/deployment.yaml) for the equivalent Pod
`securityContext` (`runAsNonRoot`, `readOnlyRootFilesystem`,
`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`,
`seccompProfile: RuntimeDefault`).

## Project layout

```
cmd/server/           main entrypoint (flags, graceful shutdown, self-healthcheck)
internal/config/      environment-based configuration
internal/httpserver/  static file handler, health handlers, security middleware
content/              default static site content (baked in as a fallback)
Dockerfile            multi-stage, distroless, non-root build
docker-compose.yml    example hardened standalone deployment
k8s/                  example hardened Kubernetes Deployment/Service
test/                 Vite + Vitest + Playwright test suite (builds & runs the
                       real container image, then exercises it over HTTP and
                       in a real headless browser)
```

## Configuration

All configuration is via environment variables (no config file parsing,
to keep the dependency surface at zero):

| Variable                                     | Default        | Purpose                                          |
| -------------------------------------------- | -------------- | ------------------------------------------------ |
| `WWWEE_ADDR`                                 | `:8080`        | Listen address                                   |
| `WWWEE_CONTENT_DIR`                          | `/srv/content` | Directory to serve                               |
| `WWWEE_TLS_CERT_FILE` / `WWWEE_TLS_KEY_FILE` | unset          | Enable TLS (both required together)              |
| `WWWEE_READ_HEADER_TIMEOUT`                  | `5s`           | Slowloris mitigation                             |
| `WWWEE_READ_TIMEOUT`                         | `10s`          | Max time to read a request                       |
| `WWWEE_WRITE_TIMEOUT`                        | `10s`          | Max time to write a response                     |
| `WWWEE_IDLE_TIMEOUT`                         | `120s`         | Keep-alive idle timeout                          |
| `WWWEE_SHUTDOWN_TIMEOUT`                     | `15s`          | Grace period for in-flight requests on `SIGTERM` |

## Mount points

- `/srv/content` (Docker `VOLUME`) — the intranet site content. Mount your
  real content here (read-only recommended); the image ships a small
  placeholder page as a fallback so the container is usable out of the box.
- Optional: mount TLS certificate/key material anywhere and point
  `WWWEE_TLS_CERT_FILE` / `WWWEE_TLS_KEY_FILE` at the mounted paths.
- CSS and JavaScript are served as ordinary static files. Same-origin
  external scripts are allowed by the default CSP; inline `<script>` blocks
  and inline event handlers are blocked by design. Put scripts in the content
  volume and reference them with `<script src="...">`.
- No log volume is used or needed — logs go to stdout as JSON lines so the
  container runtime/host/cluster logging stack owns collection and
  retention, per container-logging best practice.

## Running standalone

```sh
docker build -t wwwee-server:local .

docker run -d --name wwwee-server -p 8080:8080 \
  --read-only --cap-drop=ALL --security-opt no-new-privileges \
  --tmpfs /tmp:mode=1700 \
  -v "$PWD/content:/srv/content:ro" \
  wwwee-server:local
```

For direct HTTPS, mount a certificate and private key and change the listen
address. The certificate should include the hostname users will browse to:

```sh
docker run -d --name wwwee-server -p 8443:8443 \
  --read-only --cap-drop=ALL --security-opt no-new-privileges \
  --tmpfs /tmp:mode=1700 \
  -v "$PWD/content:/srv/content:ro" \
  -v "$PWD/certs:/run/tls:ro" \
  -e WWWEE_ADDR=:8443 \
  -e WWWEE_TLS_CERT_FILE=/run/tls/tls.crt \
  -e WWWEE_TLS_KEY_FILE=/run/tls/tls.key \
  wwwee-server:local
```

The mounted key must be readable by the container's non-root uid `65532`
(for example, use host ownership `65532:65532` and mode `0400`, or group
ownership `:65532` and mode `0440`). Kubernetes handles this through the
deployment `fsGroup` and Secret mode `0440`.

Then browse to `https://localhost:8443/` (a locally generated certificate
will produce a browser warning until it is trusted). In production, TLS may
instead terminate at an approved ingress or load balancer; the server also
supports direct TLS when that is preferable.

or with the provided hardened compose file:

```sh
docker compose up --build
```

Visit http://localhost:8080/, http://localhost:8080/healthz and
http://localhost:8080/readyz.

## Running on Kubernetes

```sh
kubectl create configmap wwwee-server-content --from-file=content
kubectl create secret tls wwwee-server-tls \
  --cert=certs/tls.crt --key=certs/tls.key
kubectl apply -f k8s/deployment.yaml -f k8s/service.yaml
```

The Kubernetes example serves HTTPS on port 443 using the `wwwee-server-tls`
Secret, with the private key mounted read-only at `/run/tls`. Use a
certificate-management system such as cert-manager or your organization's
approved PKI process to provision and rotate that Secret; do not commit
private keys to the repository.

For real (larger) site content, replace the `ConfigMap` volume in
[k8s/deployment.yaml](k8s/deployment.yaml) with a `PersistentVolumeClaim`
populated by your content publishing pipeline.

## Testing

Go vet/build:

```sh
go vet ./...
go build ./...
```

The `test/` directory contains a Vite + Vitest test suite that builds the
real Docker image, runs it with the same hardening flags as production,
and verifies it end-to-end:

- HTTP-level checks (`test/tests/health.test.ts`): health/readiness
  endpoints, security headers, method restrictions, path-traversal and
  dotfile blocking.
- Real-browser checks (`test/tests/landing.test.ts`): launches headless
  Chromium via Playwright, driven by Vitest, to confirm the landing page
  actually renders, the stylesheet loads, and unknown routes 404 — the
  same thing a person opening the intranet site in a browser would see.

```sh
cd test
npm install
npx playwright install --with-deps chromium   # one-time, downloads a browser
npm test
```

The test run builds `wwwee-server:test` and starts/stops a container
automatically; no manual server setup is required.

### Compliance scanning (OpenSCAP / GPOS SRG)

[scripts/openscap-scan.sh](scripts/openscap-scan.sh) builds the image and
scans it with OpenSCAP against DISA's GPOS SRG XCCDF profile — the profile
used to assess image-level controls in the absence of a container-specific
STIG (see [k8s/deployment.yaml](k8s/deployment.yaml) note above and the DoD
Container Hardening Process Guide §5–6). It fails unless every reported
`fail` is a documented, accepted finding.

```sh
./scripts/openscap-scan.sh
```

- Requires Docker and, on first run, network access to fetch the SCAP
  content (cached under `.cache/openscap/` afterwards).
- Writes a human-readable report to `openscap-out/report.html` and raw
  XCCDF results to `openscap-out/results.xml` (both git-ignored).
- Findings are gated against
  [scripts/openscap/accepted-findings.txt](scripts/openscap/accepted-findings.txt).
  On a fresh build of this image, 198 rules are evaluated: ~107
  not-applicable (the minimized, shell-less image has nothing for them to
  attach to), ~85 pass, and 6 accepted findings — all related to
  non-FIPS-validated crypto in the stock Go toolchain, data-in-transit
  checks (the scanned image isn't configured with TLS), and use of
  Debian's default CA trust store rather than an org-curated one. Each is
  explained, with its remediation path, in the accepted-findings file.
- The SCAP content used here is a community-maintained (Chainguard) GPOS-
  aligned datastream, not an artifact hosted by DISA itself. Treat this as
  a fast local/CI compliance signal, not a substitute for the
  authoritative DISA STIG/SRG artifacts (public.cyber.mil/stigs) required
  in a formal ATO/FedRAMP assessment.
- This scan is intentionally kept separate from `npm test`: it needs
  `--pid=host` and Docker-socket access to introspect the built image,
  which is more privileged than the rest of the hardened test suite
  requires.
