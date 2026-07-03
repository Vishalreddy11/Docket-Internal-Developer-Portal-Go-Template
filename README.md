# Docket — Internal Developer Portal Go Template

A working photo/document sharing service that demonstrates how an enterprise
Go application talks to **Postgres, MongoDB, NATS, Redis, MinIO (S3)**, and
emits **OpenTelemetry traces** to Jaeger — all running as containers in a
single namespace.

This repository is a **template**. Developers fork it, replace the file-sharing
business logic with their own, and ship. The wiring (config, adapters,
graceful fallback, observability, auth, Kubernetes manifests) stays the same.

---

## Table of contents

1. [What this is](#what-this-is)
2. [Architecture](#architecture)
3. [The six services, in plain terms](#the-six-services-in-plain-terms)
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
- Watch traces light up in Jaeger as a request hops across all six services.

It exists to answer one question: **"how do I wire a Go service to all the
infrastructure my platform offers?"** Forking Docket gives you that wiring
for free — you only write the business logic.

## Architecture

All six dependencies live in the **same Kubernetes namespace** so
they share a service-DNS prefix and stay isolated from other tenants.

```
                              ┌──────────────────┐
   HTTP / Route  ─────────►   │     docket       │   ◄── OTLP ── Jaeger
                              │  (Go binary)     │       Prom ── Prometheus
                              └────────┬─────────┘
                                       │
                       ┌───────────────┼─────────────────┐
                       ▼               ▼                 ▼
                 ┌──────────┐    ┌──────────┐      ┌─────────┐
                 │ Postgres │    │ MongoDB  │      │  NATS   │
                 │ (audit)  │    │(metadata)│      │(events) │
                 └──────────┘    └──────────┘      └─────────┘
                       ▼               ▼                 ▼
                 ┌──────────┐    ┌──────────┐      ┌─────────┐
                 │  Redis   │    │  MinIO   │      │ Jaeger  │
                 │ (counts) │    │ (bytes)  │      │ (UI)    │
                 └──────────┘    └──────────┘      └─────────┘
```

Deployment is done by the Internal Developer Portal from the manifests in
[k8s/](k8s/) — the whole stack (app + backends + admin UIs + resource
guardrails) applies as a single unit.

---

## The six services, in plain terms

For each service below you get:
- **What it is** — one paragraph, no jargon.
- **How Docket uses it** — the specific role it plays here.
- **How to look inside** — the admin UI path and what you'll see.

Every UI is served under the tenant's `<BASE_URL>` — the Portal exposes them
on paths listed below. Credentials for each UI are provisioned by the Portal
and injected into the tenant's namespace; ask your platform team for the
values, don't hardcode them anywhere.

### 🚪 UI quick reference

| Service | UI | Path |
|---|---|---|
| Docket API | Swagger UI | `<BASE_URL>/swagger` |
| Postgres | Adminer | `<BASE_URL>/adminer` |
| MongoDB | Mongo Express | `<BASE_URL>/mongo-express` |
| NATS | NUI | `<BASE_URL>/nui` |
| Redis | Redis Commander | `<BASE_URL>/redis-commander` |
| MinIO | MinIO Console | `<BASE_URL>/minio` |
| Jaeger | Jaeger UI | `<BASE_URL>/jaeger` |

### 🎬 See one upload land in every UI

```bash
curl -X POST -H "X-API-Key: <your key>" \
  -F "file=@/etc/hosts" -F "owner=demo" -F "tags=readme,demo" \
  <BASE_URL>/files
```

Then, in order, visit each UI:

| # | UI | Look for |
|---|---|---|
| 1 | **Adminer** | New row in `file_records` — action `upload`, owner `demo`. |
| 2 | **Mongo Express** | New document in `docket.files` — with your filename, tags, timestamp. |
| 3 | **NUI** | New message on stream `DOCKET_EVENTS`, subject `docket.files.uploaded`. |
| 4 | **Redis Commander** | Nothing yet — the counter appears after your first `GET /files/{id}`. |
| 5 | **MinIO Console** | New object in bucket `docket`, name = the file's UUID. |
| 6 | **Jaeger** | New trace under service `docket` with 6-8 spans (the flame graph). |

---

### 🗃️ Postgres — the relational database

**What it is.** Postgres stores **structured records** in tables, with strong
guarantees (ACID transactions, foreign keys, SQL queries). Use it when your
data has a clear schema, needs to be exactly right, and you want to run
queries like "give me every audit row for user X in the last hour."

**How Docket uses it.** As the **audit log**. Every upload / delete inserts a
row into `file_records` — a permanent, queryable record of *who* did *what*
to *which* file and *when*.

**How to look inside — Adminer at `<BASE_URL>/adminer`.** The connection
details (server, database, user) live in the Postgres Secret provisioned by
the Portal.

**What you'll see.**
- One table: **`file_records`**. Columns: `id` (audit row ID), `file_id`
  (the file's UUID), `owner`, `file_name`, `size_bytes`, `action`
  (`upload` / `delete` / `view`), `created_at`.
- Two indexes: `idx_file_records_file_id` (for looking up a file's history)
  and `idx_file_records_created_at DESC` (for "recent activity").
- Click **Select data** on `file_records` to see every action in reverse
  chronological order.

---

### 🌿 MongoDB — the document database

**What it is.** Mongo stores **flexible JSON-shaped documents** instead of
rows. Different file types attach different metadata (a photo has EXIF, a
PDF has page counts, a Word doc has authors) — Mongo doesn't care about the
shape. Use it when your schema is open-ended and evolves over time.

**How Docket uses it.** For **file metadata**. Every upload inserts a
document into the `files` collection under the `docket` database.

**How to look inside — Mongo Express at `<BASE_URL>/mongo-express`.**

**What you'll see.**
- Database dropdown → pick **`docket`**.
- Collection: **`files`**. Each document is one file, with fields:
  `_id` (matches the file UUID), `file_name`, `content_type`, `size`,
  `owner`, `description`, `tags[]`, `extra{}`, `uploaded_at`.
- Click any document to see the full JSON. Use the search box to filter by
  `owner`, tags, etc.

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

### ⚡ Redis — the in-memory cache

**What it is.** Redis stores **small, hot, ephemeral** key-value data —
counters, session tokens, rate limits — in RAM. Reads are sub-millisecond,
but data can be lost if Redis restarts (unless persistence is configured).
Use it when speed matters more than durability.

**How Docket uses it.** For **view counters**. Every `GET /files/{id}`
increments the key `docket:views:<file-id>`.

**How to look inside — Redis Commander at `<BASE_URL>/redis-commander`.**

**What you'll see.**
- Left sidebar → connection **`local`** → **`db0`**.
- Keys named **`docket:views:<uuid>`**. Each key's value is the integer
  view count.
- Click a key to see its type (`string`), value, and TTL (currently no
  expiration).

---

### 🪣 MinIO (S3) — the object store

**What it is.** MinIO is **S3 you can run yourself**. It stores raw file
bytes (photos, PDFs, videos — anything binary) cheaply and at scale, and
serves them via HTTP. Use it for any blob larger than ~1 KB.

**How Docket uses it.** For the actual file contents. Metadata about the
file lives in Mongo/Postgres; the *bytes* live in MinIO under a bucket
called `docket`, keyed by the file's UUID.

**How to look inside — MinIO Console at `<BASE_URL>/minio`.**

**What you'll see.**
- Left sidebar → **Object Browser** → bucket **`docket`**.
- Each object is a file you uploaded, named by its UUID (no extension —
  Docket stores the original filename in Mongo, not on the object).
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
(`minio.PutObject`, `files.insert`, `query INSERT`, `nats.Publish`, etc).

**How to look inside — Jaeger UI at `<BASE_URL>/jaeger`.**

**What you'll see.**
- **Service** dropdown (top left) → pick **`docket`**.
- Click **Find Traces** — a list of every request, newest first.
- Click any trace → a flame graph. You'll see 6-8 spans per upload:
  the HTTP handler wrapping MinIO + Mongo + Postgres (multiple spans:
  pool.acquire, prepare, query) + NATS.
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

### Postgres (audit records)

| Key                  | Purpose                          |
|----------------------|----------------------------------|
| `POSTGRES_HOST`      | Postgres hostname.               |
| `POSTGRES_PORT`      | Postgres port.                   |
| `POSTGRES_USER`      | DB user (Portal-provisioned).    |
| `POSTGRES_PASSWORD`  | DB password (Portal-provisioned).|
| `POSTGRES_DB`        | DB name.                         |
| `POSTGRES_SSLMODE`   | `disable` / `require` / `verify-full`. |

### MongoDB (file metadata)

| Key         | Purpose                |
|-------------|------------------------|
| `MONGO_URI` | Connection URI.        |
| `MONGO_DB`  | Database name.         |

### NATS JetStream (upload events)

| Key                    | Purpose                                              |
|------------------------|------------------------------------------------------|
| `NATS_URL`             | Connection URL.                                      |
| `NATS_STREAM`          | JetStream name (auto-created on startup).            |
| `NATS_SUBJECT_PREFIX`  | Events publish to `<prefix>.uploaded` / `.deleted`.  |

### Redis (view counters)

| Key              | Purpose             |
|------------------|---------------------|
| `REDIS_ADDR`     | `host:port`.        |
| `REDIS_PASSWORD` | Optional password.  |
| `REDIS_DB`       | Redis DB index.     |

### MinIO / S3 (file bytes)

| Key                | Purpose                              |
|--------------------|--------------------------------------|
| `MINIO_ENDPOINT`   | S3 endpoint (no scheme).             |
| `MINIO_ACCESS_KEY` | Access key (Portal-provisioned).     |
| `MINIO_SECRET_KEY` | Secret key (Portal-provisioned).     |
| `MINIO_BUCKET`     | Auto-created on startup if missing.  |
| `MINIO_USE_SSL`    | `true` to talk to HTTPS S3.          |

### OpenTelemetry (tracing)

| Key                            | Purpose                                                  |
|--------------------------------|----------------------------------------------------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT`  | OTLP HTTP endpoint. Empty disables tracing.              |
| `OTEL_SERVICE_NAME`            | Service name shown in Jaeger.                            |
| `OTEL_TRACES_SAMPLER`          | `always_on` or `always_off`.                             |

---

## In-memory fallback

Every backend adapter exposes an interface, with **two** implementations: the
real one (e.g. `MinIO`) and an in-memory one (a Go map). At startup each
adapter probes its backend with a short timeout; on failure it logs a loud
`WARN` and returns the memory implementation instead.

```
WARN minio unreachable, falling back to in-memory storage;
     uploaded files will NOT survive restart
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
| `GET`  | `/files/{id}`              | —    | File metadata + view count (increments Redis).  |
| `GET`  | `/files/{id}/download`     | —    | Stream file bytes from MinIO.                   |
| `GET`  | `/files/{id}/audit`        | —    | Audit trail from Postgres.                      |
| `POST` | `/files`                   | ✅   | Multipart upload — touches all 5 backends.      |
| `DELETE`| `/files/{id}`             | ✅   | Remove file from MinIO + Mongo + publish event. |
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
2. The handler calls `Storage.Put` → MinIO. That call gets its own child span.
3. Then `Metadata.Insert` → Mongo. Another span.
4. Then `Records.Insert` → Postgres. Another span.
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
│   ├── cache/                 — Redis interface + redis.go + memory.go
│   ├── config/                — env-var loading (every os.Getenv lives here)
│   ├── events/                — NATS interface + nats.go + memory.go
│   ├── logging/               — slog JSON + request-id context
│   ├── metadata/              — Mongo interface + mongo.go + memory.go
│   ├── metrics/               — Prometheus counters
│   ├── otel/                  — OTLP HTTP exporter setup
│   ├── records/               — Postgres interface + postgres.go + memory.go
│   └── storage/               — MinIO interface + minio.go + memory.go
├── migrations/                — SQL schema (also auto-applied at startup)
├── k8s/                       — Manifests for namespace, app, all backends
└── .env.example               — every env var the app reads
```

Each adapter package follows the same shape — `<name>.go` (the interface and
`New()` constructor), `<backend>.go` (real implementation), `memory.go`
(in-memory fallback). To replace a backend (say, swap MinIO for AWS S3 SDK),
write a new file alongside `minio.go` and switch on it in `New()`.

---

## Forking this template

What to keep:
- All of [`internal/config`](internal/config/), [`internal/logging`](internal/logging/),
  [`internal/metrics`](internal/metrics/), [`internal/otel`](internal/otel/),
  and [`internal/api/middleware.go`](internal/api/middleware.go).
- The adapter pattern in `internal/{storage,metadata,records,events,cache}`.
- The `k8s/` manifests.

What to replace:
- The `Meta` / `Record` / `Event` types — model your own domain.
- The handlers in [`internal/api/handlers.go`](internal/api/handlers.go) — your
  business logic.
- The OpenAPI spec in [`internal/api/openapi.go`](internal/api/openapi.go).
- The module path `github.com/example/docket` → your real org path.

That's it — you now have a service that connects to every piece of platform
infrastructure with sensible defaults, graceful degradation, and traces from
day one.
