# Docket — Internal Developer Portal Go Template

A working photo/document sharing service that demonstrates how an enterprise
Go application talks to **Postgres, MongoDB, NATS, Redis, MinIO (S3)**, and
emits **OpenTelemetry traces** to Jaeger — all running as containers in a
single namespace.

This repository is a **template**. Developers fork it, replace the file-sharing
business logic with their own, and ship. The wiring (config, adapters,
graceful fallback, observability, auth, Docker/K8s manifests) stays the same.

---

## Table of contents

1. [What this is](#what-this-is)
2. [Architecture](#architecture)
3. [Quick start: docker-compose](#quick-start-docker-compose)
4. [Quick start: minikube](#quick-start-minikube)
5. [The six services, in plain terms](#the-six-services-in-plain-terms)
6. [Environment variables — every key the app reads](#environment-variables--every-key-the-app-reads)
7. [In-memory fallback (why your laptop still works offline)](#in-memory-fallback)
8. [API endpoints](#api-endpoints)
9. [How tracing works](#how-tracing-works)
10. [Metrics and logs](#metrics-and-logs)
11. [Authentication](#authentication)
12. [Project layout](#project-layout)
13. [Forking this template](#forking-this-template)

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

All six dependencies live in the **same Kubernetes namespace** (`docket`) so
they share a service-DNS prefix and stay isolated from other tenants.

```
                              ┌──────────────────┐
   HTTP / 8080  ──────────►   │     docket      │   ◄── OTLP /4318 ── Jaeger
                              │  (Go binary)     │       Prom /9090  ── Prometheus
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

## Quick start: docker-compose

```bash
# 1. Build the image and spin up everything.
docker compose up -d --build

# 2. Seed 20 demo files.
curl -X POST -H "X-API-Key: dev-secret-change-me" \
  'http://localhost:8080/seed?n=20'

# 3. Upload a real file.
curl -X POST -H "X-API-Key: dev-secret-change-me" \
  -F "file=@/path/to/photo.jpg" \
  -F "owner=alice" \
  -F "tags=demo,important" \
  http://localhost:8080/files

# 4. Hit it with a load test.
curl -X POST -H "X-API-Key: dev-secret-change-me" \
  'http://localhost:8080/loadtest?n=1000&concurrency=50'

# 5. Open the UIs.
open http://localhost:8080/swagger    # Swagger UI
open http://localhost:16686           # Jaeger traces
open http://localhost:9001            # MinIO console (minioadmin/minioadmin)
```

## Quick start: Kubernetes (minikube, kind, OpenShift, etc.)

The [k8s/](k8s/) manifests are the source of truth for how Docket is deployed
to a real cluster. Everything — the app, the six backend services, the four
admin UIs, resource guardrails — lives here.

```bash
# 1. Build the image into your cluster's registry
minikube start                         # or `kind create cluster`
eval $(minikube docker-env)
docker build -t docket:dev .

# 2. Apply every manifest in order
kubectl apply -f k8s/

# 3. Watch it come up (~30 seconds)
kubectl -n docket get pods -w
```

That gives you **11 pods** in the `docket` namespace: the app, six backends,
four admin UIs.

### Everything is on NodePort — reach it from your laptop

Every service is exposed via a `NodePort` in the `30000–30999` range so you
can hit them from your browser or a local Go binary **without any
port-forwarding**. Use any cluster node's IP as the host:

```bash
NODE_IP=$(minikube ip)               # or `kubectl get nodes -o wide` on other clusters
```

| Service | NodePort | Open in browser / dial from Go |
|---|---|---|
| Docket API (Swagger) | 30080 | http://$NODE_IP:30080/swagger |
| Adminer (Postgres UI) | 30081 | http://$NODE_IP:30081 |
| Mongo Express | 30082 | http://$NODE_IP:30082 |
| NUI (NATS UI) | 30083 | http://$NODE_IP:30083 |
| Redis Commander | 30084 | http://$NODE_IP:30084 |
| Jaeger UI | 30686 | http://$NODE_IP:30686 |
| MinIO Console | 30901 | http://$NODE_IP:30901 |
| Postgres (raw) | 30432 | `$NODE_IP:30432` |
| MongoDB (raw) | 30017 | `$NODE_IP:30017` |
| NATS client (raw) | 30422 | `$NODE_IP:30422` |
| NATS monitor (HTTP) | 30822 | http://$NODE_IP:30822 |
| Redis (raw) | 30379 | `$NODE_IP:30379` |
| MinIO S3 API | 30900 | http://$NODE_IP:30900 |
| Jaeger OTLP HTTP | 30318 | http://$NODE_IP:30318 |

---

## Running the Go binary locally against a deployed stack

When Docket is deployed by the **Internal Developer Portal**, the six backend
services always run in a Kubernetes cluster — never on your laptop. If you
still want to iterate on the Go code locally (fast edit-test loop, native
debugger, etc.) while talking to the *real* deployed backends, use the
NodePorts:

```bash
export NODE_IP=$(minikube ip)   # or: kubectl get nodes -o wide | awk 'NR==2 {print $6}'

# Point every backend env var at the cluster's NodePorts
export POSTGRES_HOST=$NODE_IP  POSTGRES_PORT=30432
export MONGO_URI=mongodb://$NODE_IP:30017
export NATS_URL=nats://$NODE_IP:30422
export REDIS_ADDR=$NODE_IP:30379
export MINIO_ENDPOINT=$NODE_IP:30900
export OTEL_EXPORTER_OTLP_ENDPOINT=http://$NODE_IP:30318
export DOCKET_API_KEY=dev-secret-change-me

# Stop the in-cluster Docket app so you're the only one on the DBs
kubectl -n docket scale deploy/docket --replicas=0

# Run your local binary — same DBs, same NATS stream, same Jaeger tenant
go run ./cmd/docket
```

Your Go binary now hits the deployed Postgres, Mongo, NATS, Redis, MinIO
directly (via NodePort → cluster network → pod). Traces still land in the
deployed Jaeger and show up at http://$NODE_IP:30686. You can edit code,
`Ctrl-C`, restart — full loop in a couple of seconds.

**When you're done** and want the in-cluster app back:

```bash
kubectl -n docket scale deploy/docket --replicas=1
```

### Env var cheat sheet

Every backend key Docket reads is namespaced by service. Copy this block into
your shell profile if you iterate often:

```bash
docket_local_env() {
  export NODE_IP="${1:-$(minikube ip 2>/dev/null)}"
  export POSTGRES_HOST=$NODE_IP  POSTGRES_PORT=30432
  export MONGO_URI=mongodb://$NODE_IP:30017
  export NATS_URL=nats://$NODE_IP:30422
  export REDIS_ADDR=$NODE_IP:30379
  export MINIO_ENDPOINT=$NODE_IP:30900
  export OTEL_EXPORTER_OTLP_ENDPOINT=http://$NODE_IP:30318
  export DOCKET_API_KEY=dev-secret-change-me
  echo "docket env pointed at $NODE_IP"
}
# usage:   docket_local_env             # auto-detect minikube
#          docket_local_env 10.0.0.42   # explicit node IP
```

---

## The six services, in plain terms

For each service below you get:
- **What it is** — one paragraph, no jargon.
- **How Docket uses it** — the specific role it plays here.
- **How to look inside** — the admin UI URL, login, and what you'll actually see.

Assuming you ran `docker compose up -d` locally, all UIs are on `localhost`.
When Docket is deployed by the Internal Developer Portal, the same UIs are
reachable at `<BASE_URL>:<port>` — the exact host depends on the environment.

### 🚪 UI quick reference

| Service | UI | URL | Login |
|---|---|---|---|
| Docket API | Swagger UI | http://localhost:8080/swagger | API key `dev-secret-change-me` (pre-filled) |
| Postgres | Adminer | http://localhost:8081 | System `PostgreSQL`, server `postgres`, user/pass/DB `docket` |
| MongoDB | Mongo Express | http://localhost:8082 | `admin` / `admin` |
| NATS | NUI | http://localhost:8083 | none — add connection `nats://nats:4222` once |
| Redis | Redis Commander | http://localhost:8084 | none |
| MinIO | MinIO Console | http://localhost:9001 | `minioadmin` / `minioadmin` |
| Jaeger | Jaeger UI | http://localhost:16686 | none — pick service `docket` |

### 🎬 See one upload land in every UI

```bash
# Trigger one upload
curl -X POST -H "X-API-Key: dev-secret-change-me" \
  -F "file=@/etc/hosts" -F "owner=demo" -F "tags=readme,demo" \
  http://localhost:8080/files
```

Then, in order, visit each UI:

| # | UI | Look for |
|---|---|---|
| 1 | **Adminer** (8081) | New row in `file_records` — action `upload`, owner `demo`. |
| 2 | **Mongo Express** (8082) | New document in `docket.files` — with your filename, tags, timestamp. |
| 3 | **NUI** (8083) | New message on stream `DOCKET_EVENTS`, subject `docket.files.uploaded`. |
| 4 | **Redis Commander** (8084) | Nothing yet — the counter appears after your first `GET /files/{id}`. |
| 5 | **MinIO Console** (9001) | New object in bucket `docket`, name = the file's UUID. |
| 6 | **Jaeger** (16686) | New trace under service `docket` with 6-8 spans (the flame graph). |

---

### 🗃️ Postgres — the relational database

**What it is.** Postgres stores **structured records** in tables, with strong
guarantees (ACID transactions, foreign keys, SQL queries). Use it when your
data has a clear schema, needs to be exactly right, and you want to run
queries like "give me every audit row for user X in the last hour."

**How Docket uses it.** As the **audit log**. Every upload / delete inserts a
row into `file_records` — a permanent, queryable record of *who* did *what*
to *which* file and *when*.

**How to look inside — Adminer at http://localhost:8081**

Login form on first visit:
- **System**: `PostgreSQL`
- **Server**: `postgres`
- **Username**: `docket`
- **Password**: `docket`
- **Database**: `docket`

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

**How to look inside — Mongo Express at http://localhost:8082**

Browser will prompt for basic auth:
- **Username**: `admin`
- **Password**: `admin`

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

**How to look inside — NUI at http://localhost:8083**

NUI ships without a preconfigured connection. First time only:

1. Click **Add Connection** (top-left).
2. **Name**: anything (`docket-nats` is fine).
3. **Hosts**: `nats://nats:4222` (that's the container's DNS name inside
   the compose network — NUI runs in the same network so this resolves).
4. Save, then click the connection.

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

**How to look inside — Redis Commander at http://localhost:8084**

No login.

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

**How to look inside — MinIO Console at http://localhost:9001**

Login:
- **Username**: `minioadmin`
- **Password**: `minioadmin`

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

**How to look inside — Jaeger UI at http://localhost:16686**

No login.

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
[internal/config/config.go](internal/config/config.go). Copy `.env.example`
to `.env` and edit. In Kubernetes, these come from the
[`docket-config` ConfigMap](k8s/10-docket-config.yaml) (non-secret) and
[`docket-secrets` Secret](k8s/10-docket-config.yaml) (secret).

### Application

| Key                | Default        | Purpose                                                       |
|--------------------|----------------|---------------------------------------------------------------|
| `DOCKET_PORT`     | `8080`         | HTTP listen port.                                             |
| `DOCKET_API_KEY`  | *(empty)*      | API key for write endpoints. **Empty disables auth** (with a startup WARN). |
| `LOG_LEVEL`        | `info`         | `debug` / `info` / `warn` / `error`.                          |

### Postgres (audit records)

| Key                  | Default     | Purpose                          |
|----------------------|-------------|----------------------------------|
| `POSTGRES_HOST`      | `localhost` | Postgres hostname.               |
| `POSTGRES_PORT`      | `5432`      | Postgres port.                   |
| `POSTGRES_USER`      | `docket`   | DB user.                         |
| `POSTGRES_PASSWORD`  | `docket`   | DB password.                     |
| `POSTGRES_DB`        | `docket`   | DB name.                         |
| `POSTGRES_SSLMODE`   | `disable`   | `disable` / `require` / `verify-full`. |

### MongoDB (file metadata)

| Key         | Default                       | Purpose                |
|-------------|-------------------------------|------------------------|
| `MONGO_URI` | `mongodb://localhost:27017`   | Connection URI.        |
| `MONGO_DB`  | `docket`                     | Database name.         |

### NATS JetStream (upload events)

| Key                     | Default                    | Purpose                                          |
|-------------------------|----------------------------|--------------------------------------------------|
| `NATS_URL`              | `nats://localhost:4222`    | Connection URL.                                  |
| `NATS_STREAM`           | `DOCKET_EVENTS`            | JetStream name (auto-created on startup).        |
| `NATS_SUBJECT_PREFIX`   | `docket.files`             | Events publish to `<prefix>.uploaded` / `.deleted`. |

### Redis (view counters)

| Key              | Default          | Purpose             |
|------------------|------------------|---------------------|
| `REDIS_ADDR`     | `localhost:6379` | `host:port`.        |
| `REDIS_PASSWORD` | *(empty)*        | Optional password.  |
| `REDIS_DB`       | `0`              | Redis DB index.     |

### MinIO / S3 (file bytes)

| Key                | Default          | Purpose                              |
|--------------------|------------------|--------------------------------------|
| `MINIO_ENDPOINT`   | `localhost:9000` | S3 endpoint (no scheme).             |
| `MINIO_ACCESS_KEY` | `minioadmin`     | Access key.                          |
| `MINIO_SECRET_KEY` | `minioadmin`     | Secret key.                          |
| `MINIO_BUCKET`     | `docket`        | Auto-created on startup if missing.  |
| `MINIO_USE_SSL`    | `false`          | `true` to talk to HTTPS S3.          |

### OpenTelemetry (tracing)

| Key                            | Default             | Purpose                                                  |
|--------------------------------|---------------------|----------------------------------------------------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT`  | *(empty = disabled)*| OTLP HTTP endpoint, e.g. `http://jaeger:4318`. Empty disables tracing. |
| `OTEL_SERVICE_NAME`            | `docket`           | Service name shown in Jaeger.                            |
| `OTEL_TRACES_SAMPLER`          | `always_on`         | `always_on` or `always_off`.                             |

---

## In-memory fallback

Every backend adapter exposes an interface, with **two** implementations: the
real one (e.g. `MinIO`) and an in-memory one (a Go map). At startup each
adapter tries to connect with three retries; on final failure it logs a loud
`WARN` and returns the memory implementation instead.

```
WARN minio unreachable, falling back to in-memory storage;
     uploaded files will NOT survive restart
```

Practical effects:

- `docker compose up docket` (no backends) still boots — useful for local
  iteration.
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

Live OpenAPI spec at `/openapi.json`, Swagger UI at `/swagger`.

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
   `OTEL_EXPORTER_OTLP_ENDPOINT` using the OTLP/HTTP protocol on port `4318`.
7. Open `http://localhost:16686`, pick service `docket`, and you'll see a
   flame-graph showing exactly which backend was slow.

If `OTEL_EXPORTER_OTLP_ENDPOINT` is empty or the endpoint is unreachable,
tracing is silently disabled — the app keeps running. Observability must
never block business code.

To send traces to a different backend (Grafana Tempo, SigNoz, Datadog, etc.)
just change the endpoint. **No app code changes.**

---

## Metrics and logs

**Metrics** — Prometheus scrape at `/metrics`. Custom counters:

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
├── Dockerfile                 — distroless static multi-stage build
├── docker-compose.yml         — local dev: app + all 6 backends
├── Makefile                   — build / run / up / down / k8s-apply
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
- The Dockerfile and `k8s/` manifests.

What to replace:
- The `Meta` / `Record` / `Event` types — model your own domain.
- The handlers in [`internal/api/handlers.go`](internal/api/handlers.go) — your
  business logic.
- The OpenAPI spec in [`internal/api/openapi.go`](internal/api/openapi.go).
- The module path `github.com/example/docket` → your real org path.

One-line rebrand:

```bash
grep -rl "github.com/example/docket" . | xargs sed -i '' 's|github.com/example/docket|github.com/YOUR-ORG/YOUR-APP|g'
```

That's it — you now have a service that connects to every piece of platform
infrastructure with sensible defaults, graceful degradation, and traces from
day one.
