# boiler-plate-be-api

Beyondtech's backend service boilerplate (Go + Echo, layered architecture).
Already wired up: **Postgres** (GORM), **Redis** (cache), **OpenTelemetry**
(tracing + metrics), and an **error contract** (consistent error response format).

This repo isn't meant to run as-is — clone/fork it, then follow the
"Adapting to a New Service" guide below before writing any feature code.

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
git clone git@github.com:Beyondtech-ID/<new-service-name>.git
cd <new-service-name>
cp .env .env.local   # or edit .env directly with your local credentials
go mod download
go run cmd/main.go
```

Check it's running: `GET /health` — it also pings the DB, not just a static "OK".

## Adapting to a New Service

If this repo is used as the base for a new service (e.g. `pg-xyz-api`), change
the following. Everything is a find-and-replace — no logic needs to be rewritten.

1. **Module path (`go.mod`)**
   ```bash
   OLD="github.com/Beyondtech-ID/boiler-plate-be-api"
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
import "github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared/errors"

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
  import otelshared "github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared/otel"

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
