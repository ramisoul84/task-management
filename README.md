# Task Management API

[![CI](https://github.com/ramisoul84/task-management/actions/workflows/ci.yml/badge.svg)](https://github.com/ramisoul84/task-management/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)
![MySQL](https://img.shields.io/badge/MySQL-8.4-4479A1?logo=mysql)
![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis)

A REST API for managing tasks inside teams. It has role-based access control, a full change history, comments, Redis-cached task lists and SQL analytics. It is written in Go with Fiber, MySQL and Redis.

- **Teams & roles**: owner / admin / member, with permissions enforced in the business layer, so no data leaks between teams
- **Tasks**: status, priority, due dates, assignee; filtering, search, sorting, offset **and cursor** pagination
- **Audit trail**: every change records old and new values, author and time, written atomically with the change
- **Concurrency-safe updates**: row locks + optimistic versioning; concurrent writers get `409`, never a lost update
- **Caching**: team task lists in Redis (5 min TTL) with race-free, O(1) invalidation
- **Analytics**: one CTE query per team report (status counts, top performers, average close time, comments, overdue)
- **Production concerns**: JWT + rotating refresh tokens, rate limiting, Redis circuit breaker, structured logs with request IDs, Prometheus metrics, health probes, graceful shutdown

---

## Contents

- [Quick start](#quick-start)
- [Local development](#local-development)
- [Configuration](#configuration)
- [Database migrations](#database-migrations)
- [API](#api)
- [Roles and permissions](#roles-and-permissions)
- [Architecture](#architecture)
- [Testing](#testing)
- [Project layout](#project-layout)

---

## Quick start

Requirements: Docker with Compose v2.

```bash
cp .env.example .env
# edit .env: set DB_PASSWORD, REDIS_PASSWORD and SECRET (openssl rand -base64 48)
docker compose up -d --build --wait
```

| URL | |
|---|---|
| http://localhost:8080/api/v1 | API |
| http://localhost:8080/docs | Swagger UI |
| http://localhost:8080/readyz | Readiness (MySQL + Redis) |
| http://localhost:8080/metrics | Prometheus metrics |

Migrations run automatically on startup. To add Prometheus on `:9090`, run `make monitoring`.

## Local development

Requirements: Go 1.26+, Docker (for MySQL/Redis and integration tests).

```bash
cp .env.example .env.dev     # `make run` loads .env.dev
cp .env.example .env         # docker compose reads .env
make infra                   # MySQL + Redis in Docker, published on localhost
make run                     # API on :8080 with console logs
```

`make help` lists every target. The ones you'll use most:

| Target | |
|---|---|
| `make test` / `make race` | Unit tests (with the race detector) |
| `make test-integration` | Integration + end-to-end tests against real MySQL (testcontainers) |
| `make lint` | golangci-lint |
| `make docs` | Regenerate the OpenAPI spec from handler annotations |
| `make migration name=add_x` | Create a new migration pair |
| `make image` | Build the Docker image tagged with the git version |

## Configuration

All configuration comes from environment variables (nothing is hardcoded); [`.env.example`](.env.example) documents every one. On startup the config is validated and **all** problems are reported together:

```
error: invalid configuration:
DB_PASSWORD is required
HTTP_PORT must be an integer, got "eighty"
ALLOW_CREDENTIALS=true cannot be combined with ALLOWED_ORIGINS=*
```

The most important variables:

| Variable | Default | Notes |
|---|---|---|
| `APP_ENV` | `development` | `production` enforces a ≥32-byte `SECRET` and Secure cookies |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | required | |
| `SECRET` | required | JWT signing key |
| `REDIS_ADDR` / `REDIS_PASSWORD` | `localhost:6379` / empty | |
| `ALLOWED_ORIGINS`, `ALLOW_CREDENTIALS` | `*`, `false` | browser clients using the refresh cookie need credentials + explicit origins |
| `TRUSTED_PROXIES` | empty | only these may set `X-Forwarded-For` (otherwise clients could spoof IPs to dodge rate limits) |
| `CACHE_TASK_LIST_TTL` | `5m` | |
| `GLOBAL_RATE_MAX` / `AUTH_RATE_MAX` | `100` / `20` per minute per IP | |
| `REDIS_BREAKER_FAILURES` / `REDIS_BREAKER_OPEN_TIMEOUT` | `5` / `15s` | circuit breaker |
| `MIGRATE_ON_START` | `true` | set `false` to run migrations as a separate deploy step |
| `ENABLE_DOCS` / `ENABLE_METRICS` | `true` | |

## Database migrations

SQL migrations live in [`migrations/`](migrations) and are applied with [golang-migrate](https://github.com/golang-migrate/migrate). golang-migrate takes a MySQL advisory lock, so starting several replicas at once is safe.

```bash
task-management migrate up        # or: make migrate-up
task-management migrate down 1    # roll back one step
task-management migrate version
```

The `down` migrations are tested: the whole chain round-trips `up → down → up`.

## API

Base URL `/api/v1`. Protected endpoints need `Authorization: Bearer <access_token>`. The full contract, with models and errors, is in Swagger at `/docs` ([`docs/swagger.yaml`](docs/swagger.yaml)).

### Endpoints

| Method | Path | Who |
|---|---|---|
| **Auth** | | |
| POST | `/register` · `/auth/register` | public |
| POST | `/login` · `/auth/login` | public: returns an access token, sets an HttpOnly refresh cookie |
| POST | `/auth/refresh` | refresh cookie: rotates it (each refresh token works once) |
| POST | `/auth/logout` | revokes the access token and the refresh token |
| **Users** | | |
| GET / PATCH | `/users/me` | self |
| PUT | `/users/me/password` | self (requires the current password) |
| **Teams** | | |
| POST / GET | `/teams` | any user / own teams only |
| GET | `/teams/{id}` | member |
| PATCH | `/teams/{id}` | owner, admin |
| DELETE | `/teams/{id}` | owner |
| POST | `/teams/{id}/invite` | owner, admin (by `user_id` or `email`, role `admin`/`member`) |
| POST | `/teams/{id}/leave` | members (not the owner) |
| POST | `/teams/{id}/transfer-ownership` | owner |
| GET | `/teams/{id}/members` | member |
| PATCH | `/teams/{id}/members/{userID}/role` | owner, admin |
| DELETE | `/teams/{id}/members/{userID}` | owner, admin |
| GET | `/teams/{id}/stats` | owner, admin |
| **Tasks** | | |
| POST | `/tasks` | member of the team |
| GET | `/tasks?team_id=…` | member |
| GET | `/tasks/{id}` | member |
| PUT | `/tasks/{id}` | see [permissions](#roles-and-permissions) |
| DELETE | `/tasks/{id}` | owner, admin, task creator |
| GET | `/tasks/{id}/history` | member |
| **Comments** | | |
| POST / GET | `/tasks/{id}/comments` | member |
| PATCH | `/tasks/{id}/comments/{commentID}` | author |
| DELETE | `/tasks/{id}/comments/{commentID}` | author, owner, admin |

There are also `/livez` (liveness, never touches dependencies), `/readyz` (MySQL + Redis), `/metrics` and `/docs`.

### Example session

```bash
API=http://localhost:8080/api/v1

curl -s $API/register -H 'Content-Type: application/json' \
  -d '{"email":"rami@example.com","password":"password123","name":"Rami Suliman"}'

TOKEN=$(curl -s $API/login -H 'Content-Type: application/json' -c cookies.txt \
  -d '{"email":"rami@example.com","password":"password123"}' | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"

TEAM=$(curl -s $API/teams -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"name":"Platform"}' | jq -r .id)

curl -s $API/teams/$TEAM/invite -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"email":"colleague@example.com","role":"member"}'

TASK=$(curl -s $API/tasks -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"team_id\":\"$TEAM\",\"title\":\"Fix login bug\",\"priority\":\"high\",\"due_at\":\"2026-12-01T17:00:00Z\"}" | jq -r .id)

# Filter, search, sort and paginate
curl -s "$API/tasks?team_id=$TEAM&status=todo&priority=high&q=login&sort=due_at&order=asc&limit=20" -H "$AUTH"

# Update: send the version you read; a stale version returns 409
curl -s -X PUT $API/tasks/$TASK -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress","version":1}'

curl -s $API/tasks/$TASK/history -H "$AUTH"
curl -s $API/teams/$TEAM/stats -H "$AUTH"

# Refresh the access token using the cookie
curl -s -X POST $API/auth/refresh -b cookies.txt -c cookies.txt
```

### Listing tasks

`GET /tasks` requires `team_id` and supports:

| Parameter | |
|---|---|
| `status`, `priority`, `assignee_id` | exact filters |
| `overdue=true` | past due and not `done`/`cancelled` |
| `q` | title search (`%` and `_` match literally) |
| `sort`, `order` | `created_at` (default), `updated_at`, `due_at` (no due date sorts last), `priority`; `asc`/`desc` |
| `limit`, `offset` | page size 1–100 (default 20) |
| `cursor` | keyset pagination: pass back `next_cursor` (time-based sorts) |

```json
{ "tasks": [ … ], "total": 42, "limit": 20, "offset": 0, "has_more": true,
  "next_cursor": "eyJzIjoiY3JlYXRlZF9hdCIs…" }
```

Prefer `cursor` over `offset` for deep paging: it stays an index range scan, and it doesn't skip or repeat tasks when rows are inserted between requests.

### Updating tasks

`PUT /tasks/{id}` is a partial update. Omitted fields stay unchanged; `"assignee_id": ""` unassigns, and `"due_at": null` clears the due date. `version` is required and must match the task's current version, so concurrent edits fail with `409 version_conflict` instead of silently overwriting each other.

### Errors

Every error has the same shape. `code` is stable and meant for programs; `error` is meant for humans:

```json
{
  "error": "request validation failed",
  "code": "validation_failed",
  "details": [{ "field": "title", "message": "must be at least 3 characters" }],
  "request_id": "0f8c2a4e-1b7d-4a53-9a41-5bd1f2a1c0de"
}
```

| Status | Codes |
|---|---|
| 400 | `invalid_input`, `validation_failed`, `assignee_not_in_team` |
| 401 | `unauthorized`, `invalid_credentials` |
| 403 | `forbidden` |
| 404 | `not_found` |
| 409 | `already_exists`, `version_conflict`, `conflict` |
| 429 | `too_many_requests` (with `Retry-After`, `X-RateLimit-*`) |
| 503 | `service_unavailable` (with `Retry-After`) |
| 500 | `internal_error`: details are logged under `request_id`, never returned |

## Roles and permissions

All rules live in one file of pure functions ([`internal/service/rbac.go`](internal/service/rbac.go)), and table-driven tests cover them exhaustively.

| Action | Owner | Admin | Member |
|---|:-:|:-:|:-:|
| See team, members, tasks, history, comments | ✓ | ✓ | ✓ |
| Create tasks, comment | ✓ | ✓ | ✓ |
| Edit / delete **any** task | ✓ | ✓ | – |
| Edit / delete own task (creator) | ✓ | ✓ | ✓ |
| Change status of a task assigned to them | ✓ | ✓ | ✓ (cannot reassign or edit content) |
| Invite members (as admin or member) | ✓ | ✓ | – |
| Change roles / remove members | ✓ | members only | – |
| View analytics | ✓ | ✓ | – |
| Rename team | ✓ | ✓ | – |
| Delete team, transfer ownership | ✓ | – | – |

Invariants:
- The owner can't be removed or demoted, and ownership is never granted by invitation. Each team always has exactly one owner.
- An assignee must belong to the task's team. When a member leaves or is removed, their tasks are unassigned in the same transaction, and the change is recorded in history.
- Outside a team, nothing is visible. Team endpoints answer `403`; task endpoints answer `404`, so task IDs can't be probed.

## Architecture

```
HTTP (Fiber) ──► middleware ──► handler ──► service ──► repository ──► MySQL
                 request id      bind +     RBAC,        SQL only,      │
                 metrics         validate   invariants,  tx-aware       │
                 access log                 transactions        cache ──┴► Redis
                 recover, timeout, CORS, rate limit, auth
```

- **Layers.** Handlers only translate HTTP. Services own authorisation, invariants and transaction boundaries. Repositories only run SQL. `pkg/*` contains reusable infrastructure with no dependency on `internal/*`.
- **Transactions.** `database.WithinTx` puts the transaction in the `context`. Repositories call `db.Q(ctx)` and join it transparently, so services can compose several repository calls atomically: create team + owner, task + history, remove member + unassign tasks.
- **Concurrency.** A task update locks the row (`SELECT … FOR UPDATE`), checks the client's version, evaluates permissions and computes the history diff against that locked, current row, then writes the task and its history in one transaction. The e2e suite fires 10 parallel updates with the same version and asserts exactly one `200` and nine `409`s.
- **Cache.** Keys embed a per-team generation counter (`tasks:{team}:g<N>:<hash of filters>`). Invalidation is a single `INCR`. A reader that missed before an invalidation stores its stale page under the old generation, where nobody looks again. Plain key deletion has a race that would serve that stale page for the full TTL.
- **Resilience.** A circuit breaker hook on the Redis client fails fast during an outage. Task lists fall back to MySQL, the rate limiter fails open, and token checks fail closed with a retryable `503`.
- **Analytics.** `/teams/{id}/stats` is a single statement with CTEs, JOINs, GROUP BY, aggregates and date arithmetic, always filtered by `team_id` and gated on owner/admin. There's no N+1.
- **Security.** bcrypt, with equal-cost logins for unknown emails so timing reveals nothing; 15-minute JWTs revocable on logout; single-use rotating refresh tokens stored hashed in an HttpOnly cookie; per-IP rate limits; strict validation; security headers; IDs validated before hitting the database.
- **Observability.** One structured JSON log line per request, correlated by `X-Request-ID` (also returned in error bodies). Prometheus metrics cover requests, latency, cache hit rate, breaker state and DB pool. `/livez` and `/readyz` serve as probes.

## Testing

```bash
make test              # unit: services, RBAC policy, caches (miniredis), middleware, HTTP error contract
make test-integration  # real MySQL 8.4 via testcontainers: repositories, SQL report, end-to-end HTTP
```

- `TestIntegration_TeamStats` ([`internal/repository/integration_test.go`](internal/repository/integration_test.go)) is the integration test of the SQL report. It checks every metric against exact values, including ties, the 30-day window and data from other teams.
- The service tests use in-memory fakes that enforce MySQL's invariants (unique keys, version checks), so they test behaviour rather than scripted mocks.
- CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs lint, race tests, integration tests, an OpenAPI drift check and a Docker build.

## Project layout

```
cmd/task-management/    entrypoint: serve | migrate | healthcheck | version
internal/
  app/                  dependency wiring, lifecycle, end-to-end tests
  config/               env loading + validation
  domain/               entities, request/response models, typed errors
  service/              business logic and the RBAC policy
  repository/           MySQL access
  cache/                Redis task-list cache and token store
  server/http/          Fiber server, handlers, middleware
  metrics/              Prometheus instruments
  testutil/             testcontainers MySQL + seeding (integration tag)
pkg/
  database/             MySQL connection, DSN, context-propagated transactions
  redisx/               Redis client and circuit breaker
  logger/               zerolog setup, request-scoped loggers
  migrator/             golang-migrate wrapper
  auth/                 JWT and bcrypt
  validator/            request validation with client-friendly messages
migrations/             SQL schema migrations
docs/                   generated OpenAPI (make docs)
deploy/                 Prometheus config
```
