# template-go-web-service

An **Onklave project template**: an idiomatic Go HTTP web service you can start a
real service from. It uses only the standard library — `net/http` with Go 1.22's
enhanced `ServeMux` routing, structured logging via `log/slog`, environment-driven
config, and graceful shutdown on `SIGTERM`/`SIGINT`.

> This is a template repository. Create a new repo from it ("Use this template")
> or let Onklave provision one for you, then build your service on top.

## Layout

```
cmd/server/main.go        # process wiring: config, server, signal handling
internal/server/server.go # router + handlers (testable, no I/O on import)
internal/server/*_test.go  # handler tests
Dockerfile                # multi-stage build → distroless static, non-root
.github/workflows/ci.yml  # vet + test + build, and a docker build
```

## Routes

| Method | Path       | Response                                            |
| ------ | ---------- | --------------------------------------------------- |
| GET    | `/healthz` | `200` `{"status":"ok"}` — used by Onklave health checks |
| GET    | `/`        | `200` small JSON greeting                           |

Any other path returns `404` (Go 1.22 routing matches `/` exactly).

## Run locally

```bash
go run ./cmd/server
# listens on :8080 by default; override with PORT
PORT=9090 go run ./cmd/server

curl localhost:8080/healthz   # {"status":"ok"}
curl localhost:8080/          # {"service":"...","message":"hello from Onklave"}
```

## Test

```bash
go test ./...
go vet ./...
go build ./...
```

## Configuration

| Variable | Default | Description              |
| -------- | ------- | ------------------------ |
| `PORT`   | `8080`  | TCP port the server binds |

## How Onklave deploys it

Onklave builds the service from the included `Dockerfile` — a multi-stage build
that compiles a static, stripped binary (`CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`)
and ships it on `gcr.io/distroless/static:nonroot` (no shell, runs as non-root).

The container listens on **port 8080** and exposes a health endpoint at
**`/healthz`**. Set configuration through environment variables (e.g. `PORT`).
