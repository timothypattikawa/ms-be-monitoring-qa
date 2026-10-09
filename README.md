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

The API uses the existing `.env` settings (`APP_HOST` and `APP_PORT`; the
current local setup serves on `127.0.0.1:8080`). The worker claims PostgreSQL jobs with
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
| `QASE_API_TOKEN` | Read token sent in Qase's `Token` header |
| `QASE_BASE_URL` | Optional; defaults to `https://api.qase.io` |
| `MONITOR_MANAGER_API_KEY` | Temporary manager write gate |

No JQL configuration is needed. Bugs come from Qase's Defect API
(`GET /v1/defect/{qaseProjectCode}`), scoped per registered project code; each
defect's `external_data` carries the linked Jira key (`jira-cloud.key`), used
for a single-issue Jira lookup to enrich `assignee`/`reporter`/`priority`/`status`/`createdAt` (the
source of truth for "how critical" and "what state", not Qase's own severity/status). `environment`
is resolved from the defect's linked Qase run(s) via the same run-environment normalization used for
project runs. Defects with no linked Jira issue simply sync without those Jira-sourced fields,
falling back to Qase's own status/created timestamp. INIT status is refreshed
per-project via the same single-issue Jira lookup, not a broad JQL scan.

Register the authoritative INIT-to-Qase-project mapping before syncing a project:

```json
{
  "jiraInitKey": "INIT-386",
  "name": "Refund Alfagift",
  "qaseProjectCode": "INIT",
  "qaOwner": "QA Team",
  "stagingStartAt": "2026-09-01T00:00:00Z",
  "stagingEndAt": "2026-09-05T00:00:00Z",
  "betaStartAt": "2026-09-06T00:00:00Z",
  "betaEndAt": "2026-09-10T00:00:00Z"
}
```

Send this body to `POST /api/v1/projects` with the runtime manager key. All
fields are required. Dates use RFC3339 and each end must be at or after its
start. A repeated `jiraInitKey` returns `409`. Jira's internal issue ID is
resolved (via a single-issue lookup) and returned as `jiraInitId`; users do
not enter it. A successful create also auto-enqueues a `jira,qase` sync job
scoped to the new project, so it doesn't wait for the next scheduled sync.

**Required mapping:** Each Jira INIT needs a verified Qase project code. There
is no run ID to enter — every sync asks Qase which runs are currently
`active` for that project code and tracks all of them at once (a Qase project
typically has several parallel active runs, e.g. one per platform/scope).
A run that later closes stops counting once it's no longer in the active set;
its already-synced rows stay in the database. A Qase run title such as `[STG] AOS` supplies a
platform label; environment is a separate stored field. The worker does not
invent an INIT ↔ Qase mapping from names. Project counts are marked
`countsAvailable=false` until run membership exists for at least one active
run.

Each project response also includes `runs`. The array contains one entry per
currently-active Qase run and is empty until at least one has been synced:

```json
{
  "runId": 77,
  "title": "[STG] AOS",
  "environment": "STAGING",
  "platform": "AOS",
  "scope": "AOS",
  "ownerId": "qase-member-123",
  "passed": 27,
  "failed": 8,
  "blocked": 0,
  "total": 126,
  "startedAt": "2026-09-01T01:00:00Z",
  "finishedAt": "2026-09-01T02:00:00Z",
  "elapsedSeconds": 3600
}
```

Counts use the latest result for each case in the selected run. `ownerId` is
the stable member ID on the latest stored result and can be empty when Qase
has not supplied one. Environment values containing `STAGING`/`STG` or
`BETA` are normalized to `STAGING` or `BETA`. A recognized `[STG]`,
`[STAGING]`, or `[BETA]` run-title marker is used only when the environment
field is empty; other explicit source values are retained in uppercase.
Timestamps and elapsed time stay `null` when the source does not
provide enough data. The API does not calculate velocity, ETA, or capacity
from these values.

`GET /api/v1/projects`, `/workflow`, `/workload`, `/bugs`, `/sync-jobs`, and
`/sync-jobs/{id}` return `{asOf,sources,data}`. Source status is `fresh`,
`stale`, or `never_synced`; staleness currently means older than 24 hours.
`POST /api/v1/projects` registers a mapping and enqueues validation;
`PATCH /api/v1/projects/{id}` (manager key) edits the manual planning fields
`projectSize` and `timelinePlanDays` — send a JSON `null` to clear a value;
`POST /api/v1/sync-jobs` returns `202` and a job with steps/events.

`GET /api/v1/qa-timeline` backs the Bugs-page "QA timeline & scenario"
widget: per project it derives `qaStartAt`/`qaEndAt` from every STG-prefixed
Qase run (run start/end times, falling back to that run's result timestamps
when null), `workingDays` = calendar span minus Sat/Sun minus
`national_holidays` rows that land on weekdays. Cuti bersama is deliberately
not in the holiday table and never subtracted. The table is auto-seeded for
2025–2026 from the SKB 3 Menteri decrees at `AutoMigrate`; add later years
by inserting rows or extending `nationalHolidaySeed` in
`internal/repository/timeline.go`. Qase
sync fetches the selected run directly and pages cases and results at 100
records per request. Results use Qase's `run` filter. Run membership is
reconciled on each sync; malformed or incomplete
pages fail the job with a visible error code instead of producing partial
counts. The worker upserts by source identity, so retries do not duplicate
records. Workflow rows are cumulative selected-run state on days containing
Qase activity; `total` is the run membership. Workload daily rows are grouped
from stored Qase results. When no member display name has been synced, the
stable member ID is returned as `name`; no placeholder person is created.
Watermarks are stored for a
later incremental path. Qase's 100,000 offset
limit currently causes a safe job failure rather than silently dropping data.

### Knowledge & RAG monitoring (Solr)

Read-only view of the Solr vector DB (VPN only), under `/api/v1/knowledge`. Responses are plain JSON (no envelope).

- `GET /knowledge/overview` - `{ram:{usedGb,totalGb,pct},lastFullSync,embedding:{model,dimension},collectionsActive,autoSyncEvery}`
- `GET /knowledge/collections` - `[{name,docCount,target,coveragePct,status,lastSyncedAt,outdatedDocs,sizeBytes}]`; status `HEALTHY|OUTDATED_SYNC|NEEDS_REINDEX`
- `GET /knowledge/projects` - `{totals:{collections,docs,projects,emptyCollections},collections:[{name,label,docCount,projectCount,projects:[{code,name,docs}],error?}]}`; vector collection first, then `tc_*` by name; per-project docs from a Solr facet on `project`; names from Qase (cached 10 min), fallback to local projects table, else `""`; a failed facet gives `projects:[]` + `error:"facet_failed"`
- `GET /knowledge/documents?collection=&q=&page=&pageSize=` - `{items:[{id,title,key,sourceUrl,collection,chunks,dims,lastSyncedAt,syncStatus}],total,page,pageSize}`
- `POST /knowledge/collections/:name/sync|reindex` (manager key) - forwards `{collection,mode}` to `SOLR_SYNC_WEBHOOK_URL`; 501 `SYNC_NOT_CONFIGURED` if unset, 202 on success, 502 on failure

Env: `SOLR_BASE_URL`, `SOLR_USER`/`SOLR_PASSWORD`, `SOLR_TIMEOUT` (10s), `SOLR_VECTOR_COLLECTION`, `SOLR_COLLECTION_PREFIX` (`tc_`), `SOLR_EMBEDDING_MODEL`, `SOLR_VECTOR_DIMENSION` (fallback), `SOLR_COLLECTION_TARGETS` (`tc_apo=1200,...`, optional), `SOLR_SYNC_STALE_AFTER` (`14d`), `SOLR_AUTO_SYNC_EVERY`, `SOLR_SYNC_WEBHOOK_URL`.

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
