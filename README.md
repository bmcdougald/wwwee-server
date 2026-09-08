# wwwee-server

A minimal, hardened static web server for hosting a small department
intranet site. It is designed to run as a container — standalone with
Docker/Podman or orchestrated with Kubernetes — and to be as small and
boring as possible.

The server image contains the web server application only. **Production site
content is external to the image and is supplied at runtime from persistent
storage.** This keeps the server stateless and allows website content to be
published independently of the server image.

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
- **Externalized site content.** The image contains no production website
  files. `/srv/content` is a runtime mount point for externally managed
  persistent content and is mounted read-only by the server.
- **Kubernetes- and standalone-friendly.** Works as a bare `docker run`
  container or behind a Kubernetes `Deployment`, with `/healthz` (liveness)
  and `/readyz` (readiness) endpoints and example manifests in [k8s/](k8s).

## Deployment architecture

The intended production model separates the server application from the
website content:

```text
GitLab website project(s)
        |
        | merge to main
        v
GitLab CI/CD
  - lint / test / scan
  - publish site content
        |
        v
Persistent site storage
        |
        | mounted read-only
        v
wwwee-server container
        |
        v
/srv/content
        |
        v
Intranet users
```

The same separation applies whether the server runs under Docker or
Kubernetes:

- The **server image** contains only the compiled `wwwee-server` application
  and its runtime environment.
- **Site content** is authoritative in GitLab and is published by the content
  pipeline to persistent runtime storage.
- The server mounts that storage at `/srv/content` **read-only**.
- The server does not need GitLab credentials and does not write to the site
  content.
- Replacing or upgrading the server container does not replace the website
  content.
- Updating published site files does not require rebuilding the server image
  and does not inherently require restarting the server.

The exact persistent-storage technology is intentionally left to the
platform. Docker uses a Docker-managed volume in the supplied Compose
example; Kubernetes uses a PersistentVolumeClaim. Neither example requires a
fixed host filesystem path such as `/opt/wwwee` or `/var/www`.

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
content/              sample static site content used for local development and tests
Dockerfile            multi-stage, distroless, non-root server image
docker-compose.yml    example hardened Docker deployment using persistent content storage
k8s/                  example hardened Kubernetes Deployment/Service/PVC
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
| `WWWEE_READ_HEADER_TIMEOUT`                  | `5s`            | Slowloris mitigation                             |
| `WWWEE_READ_TIMEOUT`                         | `10s`           | Max time to read a request                       |
| `WWWEE_WRITE_TIMEOUT`                        | `10s`           | Max time to write a response                     |
| `WWWEE_IDLE_TIMEOUT`                         | `120s`          | Keep-alive idle timeout                          |
| `WWWEE_SHUTDOWN_TIMEOUT`                     | `15s`           | Grace period for in-flight requests on `SIGTERM` |

## Mount points

- `/srv/content` (Docker `VOLUME`) — the externally supplied intranet site
  content. It should be mounted read-only. The production website is **not**
  baked into the image.
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

### Docker-managed persistent volume

The supplied standalone example uses a Docker-managed named volume rather
than a host filesystem path:

```sh
docker build -t wwwee-server:local .
docker volume create wwwee-content

docker run -d --name wwwee-server -p 8080:8080 \
  --read-only --cap-drop=ALL --security-opt no-new-privileges \
  --tmpfs /tmp:mode=1700 \
  -v wwwee-content:/srv/content:ro \
  wwwee-server:local
```

A newly created volume is empty. In production, the content publishing
pipeline is responsible for populating the volume. For a local demonstration,
you can seed the Docker-managed volume from the repository's sample `content/`
directory with a temporary helper container:

```sh
docker run --rm \
  -v wwwee-content:/srv/content \
  -v "$PWD/content:/source:ro" \
  alpine sh -c 'cp -a /source/. /srv/content/'
```

The helper is only a convenient local seeding mechanism; the production
server remains read-only and does not require Alpine, Git, or any content
management tools.

### Direct HTTPS

For direct HTTPS, mount a certificate and private key and change the listen
address. The certificate should include the hostname users will browse to:

```sh
docker run -d --name wwwee-server -p 8443:8443 \
  --read-only --cap-drop=ALL --security-opt no-new-privileges \
  --tmpfs /tmp:mode=1700 \
  -v wwwee-content:/srv/content:ro \
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

### Docker Compose

The provided Compose file uses the same persistent-storage model:

```sh
docker compose up --build
```

It mounts the Docker-managed `wwwee-content` volume at `/srv/content`
read-only. Populate that volume through the content publishing process, or
seed it with the local helper command above for a demonstration. The server
image itself never needs to be rebuilt when only site content changes.

Visit http://localhost:8080/, http://localhost:8080/healthz and
http://localhost:8080/readyz.

## Running on Kubernetes

The Kubernetes example uses a PersistentVolumeClaim because the website
content is external to the server image. The Deployment has two replicas,
so the storage provider must support **ReadWriteMany (RWX)** access for the
shared content volume.

First create the content volume and populate it through your approved content
publishing pipeline:

```sh
kubectl apply -f k8s/pvc.yaml
```

Then provision TLS and deploy the server:

```sh
kubectl create secret tls wwwee-server-tls \
  --cert=certs/tls.crt --key=certs/tls.key
kubectl apply -f k8s/deployment.yaml -f k8s/service.yaml
```

The Kubernetes example serves HTTPS on port 443 using the `wwwee-server-tls`
Secret, with the private key mounted read-only at `/run/tls`. Use a
certificate-management system such as cert-manager or your organization's
approved PKI process to provision and rotate that Secret; do not commit
private keys to the repository.

The PVC is intentionally storage-provider-neutral. Do not assume that every
Kubernetes cluster provides RWX storage by default; select or provision an
approved RWX-capable storage class for your environment. The content
publishing pipeline should update the persistent volume independently of the
server Deployment.

## Testing

Go vet/build:

```sh
go vet ./...
go build ./...
```

The `test/` directory contains a Vite + Vitest test suite that builds the
real Docker image, runs it with the same hardening flags as production, and
mounts the repository's `content/` directory as an explicit read-only test
fixture. This keeps test content available without putting website content
into the production image.

It verifies the application end-to-end:

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
  a fast local/CI compliance signal, not a substitute for the authoritative
  DISA STIG/SRG artifacts (public.cyber.mil/stigs) required in a formal
  ATO/FedRAMP assessment.
- This scan is intentionally kept separate from `npm test`: it needs
  `--pid=host` and Docker-socket access to introspect the built image,
  which is more privileged than the rest of the hardened test suite
  requires.
