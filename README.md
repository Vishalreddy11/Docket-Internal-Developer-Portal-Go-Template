# Docket — Internal Developer Portal Go Template

A working photo/document sharing service that demonstrates how an enterprise
Go application talks to **Postgres, Valkey, NATS JetStream, SeaweedFS (S3)**,
and emits **OpenTelemetry traces** to Jaeger — all running as containers in a
single namespace.

All backing services are permissive open source (Apache 2.0 / BSD / MIT /
PostgreSQL License) — no AGPL, SSPL, or RSAL, no copyright encumbrances.
Safe to mirror inside an enterprise artifactory without licensing overhead.

| Image | License |
|---|---|
| `postgres:16-alpine` | PostgreSQL License |
| `nats:2.14-alpine` | Apache 2.0 |
| `valkey/valkey:9.1-alpine` | BSD 3-Clause |
| `chrislusf/seaweedfs:4.22` | Apache 2.0 |
| `jaegertracing/all-in-one:1.62` | Apache 2.0 |
| `adminer:5` | Apache 2.0 / GPL 2 (dual) |
| `ghcr.io/nats-nui/nui:0.9` | Apache 2.0 |
| `patrikx3/p3x-redis-ui` | MIT |

This repository is a **template**. Developers fork it, replace the
file-sharing business logic with their own, and ship. The wiring (config,
adapters, graceful fallback, observability, auth, Kubernetes manifests)
stays the same.

---

## Table of contents

1. [What this is](#what-this-is)
2. [Architecture](#architecture)
3. [The five services, in plain terms](#the-five-services-in-plain-terms)
4. [Environment variables — every key the app reads](#environment-variables--every-key-the-app-reads)
5. [In-memory fallback (why the app still boots when a backend is down)](#in-memory-fallback)
6. [API endpoints](#api-endpoints)
7. [How tracing works](#how-tracing-works)
8. [Metrics and logs](#metrics-and-logs)
9. [Authentication](#authentication)
10. [Project layout](#project-layout)
11. [Forking this template](#forking-this-template)

---

## What this is

Docket is a tiny but realistic backend. You can:

- Upload a file (image, PDF, text — anything).
- Get back metadata, a view counter, and an audit trail.
- See `file.uploaded` events flow through NATS JetStream.
- Watch traces light up in Jaeger as a request hops across all five services.

It exists to answer one question: **"how do I wire a Go service to all the
infrastructure my platform offers?"** Forking Docket gives you that wiring
for free — you only write the business logic.

## Architecture

All five dependencies live in the **same Kubernetes namespace** so
they share a service-DNS prefix and stay isolated from other tenants.

```
                              ┌──────────────────┐
   HTTP / Route  ─────────►   │     docket       │   ◄── OTLP ── Jaeger
                              │  (Go binary)     │       Prom ── Prometheus
                              └────────┬─────────┘
                                       │
              ┌──────────────┬─────────┼─────────┬──────────────┐
              ▼              ▼         ▼         ▼              ▼
        ┌──────────┐   ┌──────────┐   ┌────┐  ┌────────┐  ┌──────────┐
        │ Postgres │   │  Valkey  │   │NATS│  │SeaweedFS│  │  Jaeger  │
        │ records  │   │  cache   │   │bus │  │  bytes  │  │  traces  │
        │ +metadata│   │(counters)│   │    │  │ (S3 API)│  │          │
        └──────────┘   └──────────┘   └────┘  └────────┘  └──────────┘
```

Postgres serves two roles — the immutable audit log (`file_records`) and the
open-ended file metadata (`file_metadata` with a JSONB column plus a GIN
index for containment queries). Sharing one Postgres instance keeps ops
surface small while still letting metadata evolve without schema migrations.

Deployment is done by the Internal Developer Portal — the whole stack (app +
backends + admin UIs + resource guardrails) applies as a single unit. The
Platform manages all Kubernetes manifests, CI, CD, routing, and capacity;
the developer only focuses on building the application.

---

## The five services, in plain terms

For each service below you get:
- **What it is** — one paragraph, no jargon.
- **How Docket uses it** — the specific role it plays here.
- **How to look inside** — the admin UI path and what you'll see.

Every UI is served under the tenant's `<BASE_URL>` — the Portal exposes them
on paths listed below. Credentials for each UI are provisioned by the Portal
and injected into the tenant's namespace; ask your platform team for the
values, don't hardcode them anywhere.

### 🚪 UI quick reference

| Service | UI | Path | License |
|---|---|---|---|
| Docket API | Swagger UI | `<BASE_URL>/swagger` | Apache 2.0 |
| Postgres | Adminer | `<BASE_URL>/adminer` | Apache 2.0 |
| NATS | NUI | `<BASE_URL>/nui` | Apache 2.0 |
| Valkey | P3X Redis UI | `<BASE_URL>/p3x-redis-ui` | MIT |
| SeaweedFS | Filer UI (built-in) | `<BASE_URL>/seaweedfs` | Apache 2.0 |
| Jaeger | Jaeger UI | `<BASE_URL>/jaeger` | Apache 2.0 |

### 🎬 See one upload land in every UI

```bash
curl -X POST -H "X-API-Key: <your key>" \
  -F "file=@/etc/hosts" -F "owner=demo" -F "tags=readme,demo" \
  <BASE_URL>/files
```

Then, in order, visit each UI:

| # | UI | Look for |
|---|---|---|
| 1 | **Adminer → `file_records`** | New row with action `upload`, owner `demo`. |
| 2 | **Adminer → `file_metadata`** | New row — click the `doc` JSONB cell to see the file attributes, tags, timestamp. |
| 3 | **NUI** | New message on stream `DOCKET_EVENTS`, subject `docket.files.uploaded`. |
| 4 | **P3X Redis UI** | Nothing yet — the counter appears after your first `GET /files/{id}`. |
| 5 | **SeaweedFS Filer UI** | New object in bucket `docket`, name = the file's UUID. |
| 6 | **Jaeger** | New trace under service `docket` with 6-8 spans (the flame graph). |

---

### 🗃️ Postgres — relational + JSONB in one database

**What it is.** Postgres stores **structured records** in tables with strong
ACID guarantees, and — via the `JSONB` column type + GIN indexes — stores
**open-ended documents** with query performance that rivals dedicated
document stores. One database, two shapes of data.

**How Docket uses it.** Two tables in the `docket` database:

- **`file_records`** — the append-only audit log. Every upload / delete /
  view inserts a row.
- **`file_metadata`** — one row per file, with the file attributes stored
  as a JSONB `doc` column. Different file types can attach different
  fields (a photo has EXIF, a PDF has page counts) without a migration.

**How to look inside — Adminer at `<BASE_URL>/adminer`.** The connection
details (server, database, user) live in the Postgres Secret provisioned by
the Portal.

**What you'll see.**
- **`file_records`** columns: `id`, `file_id`, `owner`, `file_name`,
  `size_bytes`, `action`, `created_at`. Indexes: `idx_file_records_file_id`,
  `idx_file_records_created_at DESC`.
- **`file_metadata`** columns: `id`, `uploaded_at`, `doc` (JSONB). Indexes:
  `idx_file_metadata_uploaded_at DESC`, `idx_file_metadata_doc_gin` (GIN
  on the JSONB — enables queries like `doc @> '{"owner":"alice"}'`).

---

### 📨 NATS JetStream — the event bus

**What it is.** NATS is a **lightweight event bus** — services publish
messages, other services read them later. **JetStream** is NATS's built-in
persistence layer: events survive restart and consumers can replay them.

**How Docket uses it.** Every upload publishes a `docket.files.uploaded`
event (subject); every delete publishes `docket.files.deleted`. In
production, other services (thumbnailer, virus-scanner, search indexer)
would consume these events and do downstream work without blocking the
upload response.

**How to look inside — NUI at `<BASE_URL>/nui`.** First time only, add a
connection pointing at the in-cluster NATS service (name provided by the
Portal), then click it.

**What you'll see.**
- **Streams** tab → **`DOCKET_EVENTS`**. The stream that captures every
  `docket.files.*` subject.
- Click the stream → **Messages** → each row is one event you published,
  with the JSON body (event type, file_id, owner, timestamp, payload).
- **Consumers** tab → **`docket-log-consumer`** — the built-in consumer
  the Docket app runs (currently just logs each message). Its "delivered"
  counter climbs as events flow.

> The consumer just logs today — a real fork would replace it (or add
> more consumers) to do actual work.

---

### ⚡ Valkey — the in-memory cache

**What it is.** Valkey is the permissive-OSS fork of Redis (managed by the
Linux Foundation, BSD 3-Clause). It's **wire-protocol compatible with
Redis** — any Redis client library works unchanged — and stores small, hot,
ephemeral key-value data in RAM. Sub-millisecond reads; data can be lost on
restart unless persistence is configured. Use it when speed matters more
than durability.

**How Docket uses it.** For **view counters**. Every `GET /files/{id}`
increments the key `docket:views:<file-id>`.

**How to look inside — P3X Redis UI at `<BASE_URL>/p3x-redis-ui`.** First
time only, add a connection pointing at the in-cluster Valkey service (name
provided by the Portal).

**What you'll see.**
- Left sidebar → your connection → **`db0`**.
- Keys named **`docket:views:<uuid>`**. Each key's value is the integer
  view count.
- Click a key to see its type (`string`), value, and TTL (currently no
  expiration).

---

### 🪣 SeaweedFS — the object store (S3 API)

**What it is.** SeaweedFS is a **distributed object store** with an
S3-compatible API (Apache 2.0). It stores raw file bytes (photos, PDFs,
videos — anything binary) and serves them via HTTP. Any S3 client library
works against it unchanged.

**How Docket uses it.** For the actual file contents. Metadata about the
file lives in Postgres; the *bytes* live in SeaweedFS under a bucket
called `docket`, keyed by the file's UUID.

**How to look inside — SeaweedFS Filer UI at `<BASE_URL>/seaweedfs`** (the
filer UI ships with the SeaweedFS image — no separate container needed).

**What you'll see.**
- Bucket **`docket`** in the object listing.
- Each object is a file you uploaded, named by its UUID (no extension —
  Docket stores the original filename in the `file_metadata` row, not on
  the object).
- Click any object to preview, download, or see its metadata
  (content-type, size, last modified).

---

### 🔍 Jaeger — the trace viewer

**What it is.** Jaeger is the UI for **OpenTelemetry traces** — a
flame-graph of every operation your service performed to handle a single
request. When an endpoint feels slow, Jaeger tells you *which backend*
was slow, not just that the request was slow.

**How Docket uses it.** Every request produces one trace with a root
`docket.http` span and child spans for each backend call
(`s3.PutObject`, `pg.INSERT file_metadata`, `pg.INSERT file_records`,
`nats.Publish`, etc).

**How to look inside — Jaeger UI at `<BASE_URL>/jaeger`.**

**What you'll see.**
- **Service** dropdown (top left) → pick **`docket`**.
- Click **Find Traces** — a list of every request, newest first.
- Click any trace → a flame graph. You'll see 6-8 spans per upload:
  the HTTP handler wrapping SeaweedFS + two Postgres tables (metadata +
  records, each with pool.acquire / prepare / query spans) + NATS.
- Each span shows its duration and clickable attributes (bucket name,
  SQL text, subject, message key).

See [How tracing works](#how-tracing-works) below for the concepts.

---

## Environment variables — every key the app reads

Every key below is read via `os.Getenv` in
[internal/config/config.go](internal/config/config.go). In Kubernetes, the
Portal injects these from the tenant's
[`docket-config` ConfigMap](k8s/10-docket-config.yaml) (non-secret) and
[`docket-secrets` Secret](k8s/10-docket-config.yaml) (secret).

### Application

| Key                | Purpose                                                                     |
|--------------------|-----------------------------------------------------------------------------|
| `DOCKET_PORT`      | HTTP listen port.                                                           |
| `DOCKET_API_KEY`   | API key for write endpoints. **Empty disables auth** (with a startup WARN). |
| `LOG_LEVEL`        | `debug` / `info` / `warn` / `error`.                                        |

### Postgres (audit records + JSONB metadata)

Both `file_records` and `file_metadata` live in this Postgres instance.

| Key                  | Purpose                          |
|----------------------|----------------------------------|
| `POSTGRES_HOST`      | Postgres hostname.               |
| `POSTGRES_PORT`      | Postgres port.                   |
| `POSTGRES_USER`      | DB user (Portal-provisioned).    |
| `POSTGRES_PASSWORD`  | DB password (Portal-provisioned).|
| `POSTGRES_DB`        | DB name.                         |
| `POSTGRES_SSLMODE`   | `disable` / `require` / `verify-full`. |

### NATS JetStream (upload events)

| Key                    | Purpose                                              |
|------------------------|------------------------------------------------------|
| `NATS_URL`             | Connection URL.                                      |
| `NATS_STREAM`          | JetStream name (auto-created on startup).            |
| `NATS_SUBJECT_PREFIX`  | Events publish to `<prefix>.uploaded` / `.deleted`.  |

### Valkey (view counters)

The env vars keep the `REDIS_*` names because Valkey speaks the Redis wire
protocol and the go-redis client library uses that terminology.

| Key              | Purpose             |
|------------------|---------------------|
| `REDIS_ADDR`     | `host:port`.        |
| `REDIS_PASSWORD` | Optional password.  |
| `REDIS_DB`       | Valkey DB index.    |

### S3 (file bytes — SeaweedFS in this template)

The env vars use vendor-neutral `S3_*` names. Under the hood the Go code
uses the [MinIO Go SDK](https://github.com/minio/minio-go), which is a
generic S3 client library — swap SeaweedFS for AWS S3, Ceph RGW, or any
other S3-compatible endpoint just by changing `S3_ENDPOINT`.

| Key             | Purpose                              |
|-----------------|--------------------------------------|
| `S3_ENDPOINT`   | S3 endpoint (no scheme).             |
| `S3_ACCESS_KEY` | Access key (Portal-provisioned).     |
| `S3_SECRET_KEY` | Secret key (Portal-provisioned).     |
| `S3_BUCKET`     | Auto-created on startup if missing.  |
| `S3_USE_SSL`    | `true` to talk to HTTPS S3.          |

### OpenTelemetry (tracing)

| Key                            | Purpose                                                  |
|--------------------------------|----------------------------------------------------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT`  | OTLP HTTP endpoint. Empty disables tracing.              |
| `OTEL_SERVICE_NAME`            | Service name shown in Jaeger.                            |
| `OTEL_TRACES_SAMPLER`          | `always_on` or `always_off`.                             |

---

## In-memory fallback

Every backend adapter exposes an interface, with **two** implementations: the
real one (e.g. SeaweedFS, Postgres) and an in-memory one (a Go map). At
startup each adapter probes its backend with a short timeout; on failure it
logs a loud `WARN` and returns the memory implementation instead.

```
WARN postgres unreachable, falling back to in-memory metadata store;
     data will NOT survive restart
```

Practical effects:

- The app **still boots** when a backend is unreachable — useful for degraded
  operation and for developer iteration.
- `GET /healthz` shows per-backend mode so you can spot misconfiguration:
  ```json
  {
    "service": "docket",
    "status": "ok",
    "backends": {
      "storage":  "live",
      "metadata": "live",
      "records":  "memory",
      "events":   "live",
      "cache":    "live"
    }
  }
  ```
- The `docket_uploads_total{mode="memory"}` Prometheus counter goes up when
  you're accidentally running degraded.

---

## API endpoints

Live OpenAPI spec at `<BASE_URL>/openapi.json`, Swagger UI at `<BASE_URL>/swagger`.

| Method | Path                       | Auth | Description                                     |
|--------|----------------------------|------|-------------------------------------------------|
| `GET`  | `/healthz`                 | —    | Per-backend connection mode.                    |
| `GET`  | `/metrics`                 | —    | Prometheus scrape.                              |
| `GET`  | `/openapi.json`            | —    | OpenAPI 3 spec.                                 |
| `GET`  | `/swagger`                 | —    | Swagger UI.                                     |
| `GET`  | `/files`                   | —    | List files (paginated).                         |
| `GET`  | `/files/{id}`              | —    | File metadata + view count (increments Valkey). |
| `GET`  | `/files/{id}/download`     | —    | Stream file bytes from SeaweedFS.               |
| `GET`  | `/files/{id}/audit`        | —    | Audit trail from Postgres.                      |
| `POST` | `/files`                   | ✅   | Multipart upload — touches all 5 backends.      |
| `DELETE`| `/files/{id}`             | ✅   | Remove file + metadata + publish event.         |
| `POST` | `/seed?n=20`               | ✅   | Insert N synthetic files (max 1000).            |
| `POST` | `/loadtest?n=1000&concurrency=50` | ✅ | Fan-out load test; returns p50/p90/p95/p99. |

✅ = requires `X-API-Key: $DOCKET_API_KEY` header.

---

## How tracing works

**OpenTelemetry (OTel)** is a vendor-neutral standard for emitting traces,
metrics, and logs. A **trace** is the full story of one request — every span
(operation) it spawned, with timing and parent-child relationships.

What happens in Docket:

1. A request hits `POST /files`. The [`otelhttp` middleware](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp)
   creates a root span named `docket.http`.
2. The handler calls `Storage.Put` → SeaweedFS. That call gets its own child span.
3. Then `Metadata.Insert` → Postgres `file_metadata`. Another span.
4. Then `Records.Insert` → Postgres `file_records`. Another span.
5. Then `Events.Publish` → NATS JetStream. Another span.
6. The full tree is batched and sent to **Jaeger** at the URL in
   `OTEL_EXPORTER_OTLP_ENDPOINT` using the OTLP/HTTP protocol.
7. Open Jaeger under `<BASE_URL>/jaeger`, pick service `docket`, and you'll
   see a flame-graph showing exactly which backend was slow.

If `OTEL_EXPORTER_OTLP_ENDPOINT` is empty or the endpoint is unreachable,
tracing is silently disabled — the app keeps running. Observability must
never block business code.

To send traces to a different backend (Grafana Tempo, SigNoz, Datadog, etc.)
just change the endpoint. **No app code changes.**

---

## Metrics and logs

**Metrics** — Prometheus scrape at `<BASE_URL>/metrics`. Custom counters:

- `docket_http_requests_total{route,method,status}` — HTTP traffic.
- `docket_http_request_duration_seconds{route,method}` — latency histogram.
- `docket_uploads_total{mode}` — uploads, partitioned by `live` vs `memory`.
- `docket_events_published_total{topic}` — NATS publishes.
- `docket_cache_ops_total{op,result}` — cache operations.

**Logs** — structured JSON to stdout via `slog`. Every line emitted while
serving a request carries the same `request_id`, so you can grep for one
ID and reconstruct the full request:

```json
{"time":"2026-06-18T12:00:00Z","level":"INFO","msg":"http",
 "request_id":"7f3c…","method":"POST","route":"/files","status":201,"duration_ms":42}
```

The `X-Request-Id` response header echoes the ID so a frontend can log it
alongside its own errors.

---

## Authentication

Write endpoints require the header `X-API-Key: <value of DOCKET_API_KEY>`.
`Authorization: Bearer <value>` is also accepted. If `DOCKET_API_KEY` is
empty (the default), auth is **disabled with a startup WARN**. Set the key
for any non-local deployment.

This stub is deliberately minimal — replace with JWT/OIDC for production.
See [`internal/api/middleware.go`](internal/api/middleware.go).

---

## Project layout

```
.
├── cmd/docket/main.go        — entrypoint (config → otel → app → http server)
├── internal/
│   ├── api/                   — HTTP layer (router, handlers, middleware, openapi)
│   ├── app/                   — wires every adapter into a single struct
│   ├── cache/                 — cache interface + redis.go (Valkey) + memory.go
│   ├── config/                — env-var loading (every os.Getenv lives here)
│   ├── events/                — event bus interface + nats.go + memory.go
│   ├── logging/               — slog JSON + request-id context
│   ├── metadata/              — metadata interface + postgres.go (JSONB) + memory.go
│   ├── metrics/               — Prometheus counters
│   ├── otel/                  — OTLP HTTP exporter setup
│   ├── records/               — audit interface + postgres.go + memory.go
│   └── storage/               — object-store interface + s3.go (SeaweedFS via MinIO Go SDK) + memory.go
├── migrations/                — SQL schema (also auto-applied at startup)
├── k8s/                       — Manifests for namespace, app, all backends
└── .env.example               — every env var the app reads
```

Each adapter package follows the same shape — `<name>.go` (the interface and
`New()` constructor), `<backend>.go` (real implementation), `memory.go`
(in-memory fallback). To replace a backend (say, swap SeaweedFS for AWS S3),
you don't need to write new code — just point `S3_ENDPOINT` at the new
endpoint (the S3 SDK is generic). Only if you need a fundamentally different
API do you write a new adapter file alongside `s3.go`.

---

## Forking this template

What to keep:
- All of [`internal/config`](internal/config/), [`internal/logging`](internal/logging/),
  [`internal/metrics`](internal/metrics/), [`internal/otel`](internal/otel/),
  and [`internal/api/middleware.go`](internal/api/middleware.go).
- The adapter pattern in `internal/{storage,metadata,records,events,cache}`.

What to replace:
- The `Meta` / `Record` / `Event` types — model your own domain.
- The handlers in [`internal/api/handlers.go`](internal/api/handlers.go) — your
  business logic.
- The OpenAPI spec in [`internal/api/openapi.go`](internal/api/openapi.go).
- The module path `github.com/example/docket` → your real org path.

That's it — you now have a service that connects to every piece of platform
infrastructure with sensible defaults, graceful degradation, and traces from
day one.
