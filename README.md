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

For standalone Docker deployments, the host-side content location is supplied
by the implementer through `WWWEE_CONTENT_PATH` in `.env`. The Compose file
maps that host directory to the fixed container path `/srv/content` and
mounts it read-only. This avoids imposing a host filesystem convention and
allows the same Compose configuration to be used on macOS, Linux, or another
Docker host where the implementer has an appropriate persistent filesystem
location.

Kubernetes uses a PersistentVolumeClaim for the same purpose. The pipeline or
other approved deployment process is responsible for publishing content to
the persistent storage; the server only consumes it read-only.

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
.env.example          deployment-specific host paths for content and optional TLS material
Dockerfile            multi-stage, distroless, non-root server image
docker-compose.yml    example hardened Docker deployment using an external host content path
k8s/                  example hardened Kubernetes Deployment/Service/PVC
test/                 Vite + Vitest + Playwright test suite (builds & runs the
                       real container image, then exercises it over HTTP and
                       in a real headless browser)
```

## Configuration

The application itself is configured through environment variables (no config
file parsing, to keep the dependency surface at zero). Docker Compose also
uses a `.env` file for **deployment-specific host filesystem paths**. The
`.env` file is intentionally ignored by Git; use [.env.example](.env.example)
as the template.

### Application configuration

| Variable                                     | Default        | Purpose                                          |
| -------------------------------------------- | -------------- | ------------------------------------------------ |
| `WWWEE_ADDR`                                 | `:8080`        | Listen address                                   |
| `WWWEE_CONTENT_DIR`                          | `/srv/content` | Directory to serve                               |
| `WWWEE_TLS_CERT_FILE` / `WWWEE_TLS_KEY_FILE` | unset          | Enable TLS (both required together)              |
| `WWWEE_READ_HEADER_TIMEOUT`                  | `5s`            | Slowloris mitigation                             |
| `WWWEE_READ_TIMEOUT`                         | `10s`           | Max time to read a request                       |
| `WWWEE_WRITE_TIMEOUT`                        | `10s`           | Max time to write a response                     |
| `WWWEE_IDLE_TIMEOUT`                         | `120s`         | Keep-alive idle timeout                          |
| `WWWEE_SHUTDOWN_TIMEOUT`                     | `15s`           | Grace period for in-flight requests on `SIGTERM` |

### Deployment path configuration

`.env` contains host-side paths that vary by deployment environment:

```dotenv
# Host-side directory containing the published website content.
WWWEE_CONTENT_PATH=/path/to/wwwee/content

# Host-side TLS certificate and private-key paths.
# Used when direct HTTPS is enabled in docker-compose.yml.
WWWEE_TLS_CERT_PATH=/path/to/tls/tls.crt
WWWEE_TLS_KEY_PATH=/path/to/tls/tls.key
```

The implementer chooses paths appropriate for the host and ensures the
content directory exists and is writable by the content publishing process.
The web server receives the content directory read-only at `/srv/content`.
TLS certificate and key files are also mounted read-only when direct HTTPS is
configured.

Do not commit `.env` or private keys to the repository. `.env.example`
contains placeholders only.

## Mount points

- `/srv/content` — externally supplied intranet site content. The Docker
  Compose deployment bind-mounts the host directory selected by
  `WWWEE_CONTENT_PATH`; Kubernetes mounts the content PVC. The server mounts
  the content read-only and the production website is **not** baked into the
  image.
- `/run/tls/tls.crt` and `/run/tls/tls.key` — optional direct-HTTPS certificate
  and private key mount points. Their host-side locations are supplied by
  `WWWEE_TLS_CERT_PATH` and `WWWEE_TLS_KEY_PATH` when direct HTTPS is enabled.
- CSS and JavaScript are served as ordinary static files. Same-origin
  external scripts are allowed by the default CSP; inline `<script>` blocks
  and inline event handlers are blocked by design. Put scripts in the content
  volume and reference them with `<script src="...">`.
- No log volume is used or needed — logs go to stdout as JSON lines so the
  container runtime/host/cluster logging stack owns collection and
  retention, per container-logging best practice.

## Running standalone

### Docker with externally managed content

The standalone Docker deployment uses a **host bind mount**, not a
Docker-managed named volume. This is intentional: the website content is an
externally managed deployment artifact that the content pipeline or operator
must be able to write directly to persistent host storage.

Copy the environment template and set the paths for the deployment host:

```sh
cp .env.example .env
```

Edit `.env` so `WWWEE_CONTENT_PATH` points to a directory that the deployment
process can write. For example:

```dotenv
WWWEE_CONTENT_PATH=/path/to/wwwee/content
```

Create the directory and populate it with the site content:

```sh
mkdir -p /path/to/wwwee/content
cp -a content/. /path/to/wwwee/content/
```

Then start the server:

```sh
docker compose up --build -d
```

The container sees the host directory at `/srv/content`, read-only. Changes
to files in the host content directory are therefore visible to the running
server without rebuilding the image or restarting the container.

For example, after editing a published file:

```sh
cp -a updated-site/. /path/to/wwwee/content/
```

The server continues serving from the same mount; no image rebuild is
required.

### Direct HTTPS with Docker

Direct HTTPS uses the same `.env` deployment configuration. Set the
certificate and private-key paths:

```dotenv
WWWEE_CONTENT_PATH=/path/to/wwwee/content
WWWEE_TLS_CERT_PATH=/path/to/tls/tls.crt
WWWEE_TLS_KEY_PATH=/path/to/tls/tls.key
```

The base Compose example defaults to HTTP on port 8080. To enable direct
HTTPS, add the following TLS mounts to the service's `volumes` section in
`docker-compose.yml`:

```yaml
      - type: bind
        source: ${WWWEE_TLS_CERT_PATH}
        target: /run/tls/tls.crt
        read_only: true
      - type: bind
        source: ${WWWEE_TLS_KEY_PATH}
        target: /run/tls/tls.key
        read_only: true
```

and configure the service environment and port mapping for HTTPS:

```yaml
    ports:
      - "8443:8443"
    environment:
      WWWEE_ADDR: ":8443"
      WWWEE_CONTENT_DIR: "/srv/content"
      WWWEE_TLS_CERT_FILE: "/run/tls/tls.crt"
      WWWEE_TLS_KEY_FILE: "/run/tls/tls.key"
```

The mounted key must be readable by the container's non-root uid `65532`
(for example, use host ownership `65532:65532` and mode `0400`, or group
ownership `:65532` and mode `0440`).

Then browse to `https://localhost:8443/` (a locally generated certificate
will produce a browser warning until it is trusted). In production, TLS may
instead terminate at an approved ingress or load balancer; the server also
supports direct TLS when that is preferable.

### Docker Compose

The provided Compose file is the recommended local/standalone Docker example:

```sh
docker compose up --build -d
```

It requires `WWWEE_CONTENT_PATH` to be defined in `.env` and bind-mounts that
host directory to `/srv/content` read-only. The host path is intentionally not
hard-coded so each deployment can choose a location appropriate to its
filesystem permissions and operational model.

Visit http://localhost:8080/, http://localhost:8080/healthz and
http://localhost:8080/readyz.

To change the content location, update `WWWEE_CONTENT_PATH` in `.env`, ensure
the new directory contains the published site, and recreate the container:

```sh
docker compose down
docker compose up -d
```

No application or Dockerfile change is required to change the host content
location.

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
  content (cached under `.cache/openscap/`) afterwards.
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
