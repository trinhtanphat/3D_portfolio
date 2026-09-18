# Go Backend API

A compact production-style backend portfolio project built for demonstrating backend engineering fundamentals with **Golang, PostgreSQL, Docker, Linux-friendly containers, structured logging, health checks, metrics, tests, and CI**.

## Why this project exists

The implementation is intentionally small enough to review in an interview while still showing practices used in real services:

- Go standard-library HTTP routing and context propagation
- PostgreSQL via `database/sql` + pgx driver
- parameterized SQL and an indexed migration
- connection-pool limits and lifetimes
- readiness/health endpoints
- Prometheus-compatible text metrics
- JSON structured logs with request latency and status
- input size limits and validation
- graceful SIGINT/SIGTERM shutdown
- multi-stage, non-root container image
- Docker Compose for local PostgreSQL
- unit tests, `go vet`, race detector, formatting gate, and Docker build in CI

## API

| Method | Endpoint | Purpose |
| --- | --- | --- |
| GET | `/healthz` | Liveness + uptime |
| GET | `/readyz` | Database readiness |
| GET | `/metrics` | Request/error counters |
| GET | `/api/v1/tasks` | List latest 100 tasks |
| POST | `/api/v1/tasks` | Create a task |
| PATCH | `/api/v1/tasks/{id}/complete` | Mark a task complete |

Example:

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H 'content-type: application/json' \
  -d '{"title":"Ship the Go backend portfolio"}'
```

## Run locally

Requirements: Docker + Docker Compose.

```bash
docker compose up --build
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/metrics
```

PostgreSQL automatically applies `migrations/0001_create_tasks.sql` on a fresh volume.

## Engineering notes

### Database

The service uses bounded connection-pool settings instead of relying on unlimited defaults. Queries are parameterized, result sets are bounded, and the migration adds an index supporting the list ordering path.

### Reliability

`/healthz` is process-only liveness. `/readyz` checks PostgreSQL independently, allowing an orchestrator or load balancer to stop routing traffic when the dependency is unavailable. Shutdown is bounded to ten seconds so in-flight requests can finish.

### Observability

Every request emits a structured JSON log containing HTTP method, path, status, and duration. `/metrics` exposes Prometheus text-format counters for total requests and 5xx responses.

### Security baseline

The server limits JSON request bodies, rejects unknown JSON fields, validates task titles, uses parameterized SQL, applies HTTP timeouts, and runs the runtime image as a non-root user.

## Interview discussion points

- Why liveness and readiness are separate
- How database pool sizing affects latency and PostgreSQL
- Why bounded queries and indexes matter
- When to add transactions or idempotency keys
- How to extend metrics to histograms and per-route labels
- How horizontal replicas change database connection budgets
