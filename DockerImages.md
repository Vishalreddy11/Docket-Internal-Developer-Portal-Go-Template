# Docker Images

Every image the stack pulls, and why we picked it. Kept in sync with `docker-compose.yml` and `k8s/`.

## Production path (scanned by Trivy)

| Image | Role | Why this one |
|---|---|---|
| `docket:local` | Our Go app | Built from `./Dockerfile` — multi-stage, distroless-nonroot final image. |
| `cgr.dev/chainguard/postgres:latest` | Primary DB | Chainguard = near-zero-CVE hardened distro. Passed scan with 0 HIGH/CRITICAL. |
| `nats:2.14-alpine` | Event bus | Official NATS. Alpine base was already clean at scan time. |
| `cgr.dev/chainguard/valkey:latest` | Cache | Chainguard hardened. Valkey is the Redis-protocol-compatible LF fork. |
| `chrislusf/seaweedfs:latest` | S3 object store | Only S3-compatible image without CRITICAL findings (MinIO had 6 CRIT). |
| `jaegertracing/jaeger:latest` | Tracing UI | Jaeger v2, official. Best OSS tracing UI available today. |

## Dev-only (NOT scanned, NOT deployed)

| Image | Role |
|---|---|
| `adminer:5` | Postgres GUI (local dev only) |
| `ghcr.io/nats-nui/nui:0.9` | NATS GUI (local dev only) |
| `patrikx3/p3x-redis-ui:2026.4.3014` | Redis/Valkey GUI (local dev only) |

## Tooling

| Image | Role |
|---|---|
| `aquasec/trivy:latest` | Vuln scanner. Runs on `docker compose up trivy`. |
| `nginx:alpine` | Serves the generated Trivy reports at `http://localhost:8090`. |

## Docker Hub fallback (if org blocks `cgr.dev`)

Many orgs allow only Docker Hub or an internal Artifactory mirror of it.
Swap these two lines in `docker-compose.yml` / `k8s/`:

| Chainguard image | Docker Hub fallback | Trade-off |
|---|---|---|
| `cgr.dev/chainguard/postgres:latest` | `postgres:17-alpine` | ~1 CRIT / ~13 HIGH from alpine's OpenSSL 3.5.6-r0 + Go stdlib bundled in postgres 17. All pending upstream alpine/postgres rebuild. |
| `cgr.dev/chainguard/valkey:latest` | `valkey/valkey:9.1-alpine` | ~3 HIGH from the same OpenSSL 3.5.6-r0. |

If those extra findings are blockers:

1. Add the CVE IDs to `.trivyignore` with rationale + review date, **or**
2. Use `bitnami/postgresql:17` and `bitnami/valkey:latest` — hardened Docker Hub images maintained by Broadcom/Bitnami (worth a scan first; results vary by tag), **or**
3. Ask infra to whitelist `cgr.dev` — it's the Chainguard public registry, not a paid product for these two images.

## Selection rules

1. **Prefer Chainguard** for anything on the production path — best CVE hygiene.
2. **Fall back to official `-alpine`** if no Chainguard variant exists and the alpine tag is clean.
3. **Third-party images** (SeaweedFS, Jaeger) go through Trivy; residuals get a documented entry in `.trivyignore`.
4. **Never pin admin UIs to prod** — they widen attack surface without carrying business logic.
