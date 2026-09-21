# ms-monitoring-qa-be

Go + Echo starter for the Alfagift QA Monitoring backend.
Already wired up: **Postgres** (GORM), **Redis** (cache), **OpenTelemetry**
(tracing + metrics), and an **error contract** (consistent error response format).

The service identity is set to `ms-monitoring-qa-be`. Dashboard reads and
durable Jira/Qase sync are implemented under `/api/v1`.

## QA Monitoring API and worker

Run PostgreSQL and Redis, then start two processes:

```bash
go run ./cmd
go run ./cmd/worker
```

The API uses the existing `.env` settings (`APP_HOST=127.0.0.1`,
`APP_PORT=3002` locally). The worker claims PostgreSQL jobs with
`FOR UPDATE SKIP LOCKED`; it enqueues scheduled jobs at 08:00 and 17:00
Asia/Jakarta. `POST /api/v1/sync-jobs` enqueues a manual job and requires
`X-Manager-Key` matching `MONITOR_MANAGER_API_KEY`. If that variable is
unset, manager writes are disabled. Keep this key server side; replace this
temporary gate with the organization's authenticated manager identity before
production. Dashboard reads also need identity based access control before
production.

Set these only in runtime secrets/environment, never in Angular or Git:

| Variable | Value |
| --- | --- |
| `JIRA_BASE_URL` | Jira Cloud site origin, such as `https://example.atlassian.net` |
| `JIRA_EMAIL` | Jira account used for Basic auth |
| `JIRA_API_TOKEN` | Jira API token for that account |
| `JIRA_ACTIVE_JQL` | Approved active INIT JQL; the supplied QA filtered JQL may be used temporarily |
| `JIRA_BUG_JQL` | Approved JQL for linked bug issues; unset means Bugs remains empty |
| `QASE_API_TOKEN` | Read token sent in Qase's `Token` header |
| `QASE_BASE_URL` | Optional; defaults to `https://api.qase.io` |
| `MONITOR_MANAGER_API_KEY` | Temporary manager write gate |

Load the approved JQL directly from the supplied file at runtime. Quoting the
path is required because its filename contains spaces and parentheses:

```bash
export JIRA_ACTIVE_JQL="$(cat '/absolute/path/project = INIT AND status not in (C.txt')"
go run ./cmd/worker
```

This keeps the query out of shell history, source files, and committed `.env`
files. The Jira base URL, bug mapping JQL, and INIT-to-Qase project codes are
still required before a real sync can be complete.

**Required mapping:** Each Jira INIT needs a verified Qase project code in
`projects.qase_project_code`. Jira issue links or an approved field must
connect bugs to an INIT. The attached JQL filters to one QA account; confirm
whether that is the intended dashboard scope before treating its counts as
team wide. A Qase run title such as `[STG] AOS` supplies a platform label;
environment is a separate stored field. The worker does not invent an
INIT ↔ Qase mapping from names. Project counts are marked
`countsAvailable=false` until run membership and a unique mapping exist.

`GET /api/v1/projects`, `/workflow`, `/workload`, `/bugs`, `/sync-jobs`, and
`/sync-jobs/{id}` return `{asOf,sources,data}`. Source status is `fresh`,
`stale`, or `never_synced`; staleness currently means older than 24 hours.
`POST /api/v1/projects` saves/updates a mapping and enqueues validation;
`POST /api/v1/sync-jobs` returns `202` and a job with steps/events. Qase
backfill pages cases, runs, run membership and results at 100 records per
request. Run membership is reconciled on each sync; malformed or incomplete
pages fail the job with a visible error code instead of producing partial
counts. The worker upserts by source identity, so retries do not duplicate
records. It performs a full source reconciliation on every sync until the
tenant's Qase result timestamp timezone and Jira bug mapping are verified;
watermarks are stored for the later incremental path. Qase's 100,000 offset
limit currently causes a safe job failure rather than silently dropping data.

## Tech Stack

- **Language**: Go 1.25
- **Web framework**: [Echo v4](https://echo.labstack.com/)
- **Dependency Injection**: [Uber Dig](https://github.com/uber-go/dig)
- **ORM**: [GORM](https://gorm.io/) + PostgreSQL
- **Cache**: Redis (`go-redis/redis/v8`)
- **Observability**: OpenTelemetry (trace + metrics, exported via OTLP gRPC), Zap for logging
- **Error contract**: [errorx](https://github.com/joomcode/errorx) + [errcntrct](https://github.com/Saucon/errcntrct)
- **Config**: Viper (reads from `.env` / environment variables)

## Project Structure

```text
cmd/main.go                     # entrypoint: bootstraps DI + OTel + Echo server
configs/                        # config structs (App, DB, Cache, OTel) + loader
internal/
  controller/                   # HTTP handlers (Echo), routing
  usecase/                      # business logic
  repository/                   # database access layer
  di/                           # dependency injection wiring (dig)
  shared/
    cache/                      # cache interface + redis implementation
    delivery/                   # response helpers (success & error contract)
    dto/                        # common request/response DTOs
    errors/                     # error contract (errorx) + extract helpers
    log/                        # zap logger
    otel/                       # OTel tracing/metrics setup
    code/                       # generated from errorContract.json (errcntrct)
pkg/helper/                     # generic helpers (validation, custom types)
errorContract.json               # source of error codes for errcntrct
```

## Local Setup

Prerequisites: Go 1.25+, PostgreSQL, Redis.

```bash
cp .env .env.local   # or edit .env directly with your local credentials
go mod download
go run cmd/main.go
```

Check it's running: `GET /health` — it also pings the DB, not just a static "OK".

Current identity: Go module `github.com/Beyondtech-ID/ms-monitoring-qa-be`,
application/OTel service `ms-monitoring-qa-be`, Jenkins image
`ms-monitoring-qa-be`, and local database `ms-monitoring-qa-be-local`.
The Git remote still points to the original boilerplate repository; update it
when the new remote exists.

`errorContract.json` still contains boilerplate example codes; define real QA
Monitoring error codes when the first business endpoints are added. The CDK
stack under `infra/` still carries the old BackboneDoku identity and must be
reviewed before any deployment.

## Adapting to a New Service

If this repo is used as the base for a new service (e.g. `pg-xyz-api`), change
the following. Everything is a find-and-replace — no logic needs to be rewritten.

1. **Module path (`go.mod`)**
   ```bash
   OLD="github.com/Beyondtech-ID/ms-monitoring-qa-be"
   NEW="github.com/Beyondtech-ID/<new-service-name>"
   grep -rl "$OLD" --include="*.go" . | xargs sed -i '' "s#$OLD#$NEW#g"
   sed -i '' "s#^module $OLD#module $NEW#" go.mod
   go mod tidy
   ```

2. **Service name in `.env`** — this is automatically used as the error
   namespace name (`internal/shared/errors/errors.go` reads
   `configs.GetConfig().AppConfig.Name`), so just update the env vars, no code
   change needed:
   ```
   APP_NAME=<new-service-name>
   APP_ENV_PREFIX=<NEW_PREFIX>
   ```

3. **Service name in OTel** (hardcoded, must be changed manually) —
   `internal/shared/otel/instrumentation.go`:
   ```go
   const ServiceName = "<new-service-name>"
   const ServiceInstrumentationName = "github.com/Beyondtech-ID/<new-service-name>"
   ```

4. **Jenkinsfile** — `IMAGE_NAME` near the top of the file; update it to match
   the new docker image name.

5. **`errorContract.json`** — remove the placeholder error codes and fill in
   the new service's codes (format: `"<SERVICE>-<NNNNN>": {"var": "...", "msg": "..."}`).
   After editing, regenerate `internal/shared/code/contract.go` with
   [errcntrct](https://github.com/Saucon/errcntrct) (the constants in that file
   are meant to be generated, not hand-written).

6. **Infra (`infra/`)** — if the CDK infra is reused, check the stack name in
   `infra/lib/*.ts` still points at the old service name and update it before
   deploying. This is out of scope for the Go refactor, so it's intentionally
   left untouched here.

## Usage Guide

### Error Handling (`internal/shared/errors`)

Every business error should be created with `errors.New`, not a plain
`fmt.Errorf`, so it carries an HTTP code plus a service/case code that can be
extracted automatically into the response.

```go
import "github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/errors"

// define errors as package-level vars (same pattern already in errors.go)
var ErrAccountNotFound = errors.New(http.StatusNotFound, "21-01", "account not found")

// use it in usecase/repository
if account == nil {
    return ErrAccountNotFound
}
```

The code `"21-01"` is parsed into `serviceCode="21"` + `caseCode="01"`, and
shows up in the response as `ResponseCode: "40421-01"` (httpCode + serviceCode
+ caseCode).

Other useful helpers:

| Function | Purpose |
|---|---|
| `errors.Wrap(err, msg)` | Replace the message shown to the user, while keeping the original error as the cause (for logs/debugging) |
| `errors.WithUnderlyingMsg(err, msg)` | Attach a hidden message (`hidden: ...`) for backend traceability, never shown to the user |
| `errors.Is(err, target)` | Compare errors by their error code, not pointer identity |
| `errors.ExtractSnapError(err)` | Get `(httpCode, serviceCode, caseCode, message)` from an error — used internally by `delivery.ResponseError` |
| `errors.GetStatusCodeAndMessage(err)` | Simpler variant, just returns `(httpCode, message)` |

### Response Contract (`internal/shared/delivery`)

Every controller handler must respond through these helpers, not raw
`c.JSON`, so the response shape stays consistent across all endpoints.

```go
// success response
return delivery.ResponseWithCode(c, data, "success", http.StatusOK, "21", "00")

// error response from errors.New / errors.Wrap
if err != nil {
    return delivery.ResponseError(c, err)
}

// error response specifically for request bind/validation failures (echo.Bind)
if err := c.Bind(&req); err != nil {
    return delivery.ResponseBindError(c, err, "21", "00")
}
```

### Config (`configs`)

Add a new config section by adding a struct + field in `configs/config.go`,
then set its env vars following the `mapstructure` tag naming — it's
auto-bound, no manual registration needed. Access it via `configs.GetConfig()`.

### Cache / Redis (`internal/shared/cache`)

```go
type Dependency struct {
    dig.In
    Cache cache.Cache
}
```
`cache.Cache` is already wired to Redis through `internal/di/cache.go` — just
inject it, no need to build your own Redis client. Cache errors (`ErrSetCache`,
`ErrGetCache`, etc.) are already defined in `errors.go`.

### OpenTelemetry (`internal/shared/otel`)

- HTTP tracing and request metrics are already wired automatically via
  middleware in `cmd/main.go` (skipped for `/health`, `/metrics`, `/ready`).
- For manual spans in usecase/repository code, import the package with an
  alias (it shares its name with `go.opentelemetry.io/otel`) and use the
  tracer singleton:
  ```go
  import otelshared "github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/otel"

  ctx, span := otelshared.Tracer.Start(ctx, "usecase.DoSomething")
  defer span.End()
  ```
- The OTel collector endpoint is set via `OTEL_COLLECTORADDR` (defaults to
  `localhost:4317` if empty).

### Adding a New Usecase / Repository / Controller

The DI pattern is consistent across the three layers
(`internal/{controller,usecase,repository}/di.go`):
1. Add the interface + implementation in a new file under the relevant layer.
2. Register its constructor via `container.Provide(NewXxx)` in that layer's
   `di.go`.
3. If another layer needs it, add it as a field on that layer's `Dependency`
   struct (`dig.In`).

### Health Check

`GET /health` already checks the Postgres connection (`Ping()` on `*sql.DB`).
If you need to check Redis too, add a similar ping in
`internal/controller/health.go`.
