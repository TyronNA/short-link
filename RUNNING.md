# Running ShortLink

## Prerequisites

- **Go 1.22+** (developed on 1.25). No CGO and no external services required — the SQLite
  driver is pure Go.

## Configuration (environment variables)

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `8080` | TCP port to listen on |
| `DB_PATH` | `shortlink.db` | SQLite file path (created if absent) |
| `BASE_URL` | `http://localhost:<PORT>` | prefix used to build returned short URLs |
| `FEISTEL_KEY` | `shortlink-default-key` | secret that parameterizes the code permutation — **set this in production** |

See [.env.example](.env.example) for a documented template (the app reads env vars directly
and does **not** auto-load `.env`; use `make run`/`make start` or export the vars yourself).

> The same `FEISTEL_KEY` must be used to decode codes that were produced with it. Changing
> the key changes the `id → code` mapping (existing codes would decode to different IDs).
> Persisted URLs themselves are unaffected.

## Run locally

```bash
go run ./cmd/server
# or build a static binary:
CGO_ENABLED=0 go build -o shortlink ./cmd/server
./shortlink
```

With custom config:

```bash
PORT=9000 DB_PATH=/var/data/shortlink.db BASE_URL=https://sho.rt FEISTEL_KEY=$(openssl rand -hex 16) ./shortlink
```

The server logs `listening on :<port>` and shuts down gracefully on `SIGINT`/`SIGTERM`.

## Try it (curl)

```bash
# Encode
curl -s -X POST localhost:8080/encode \
  -d '{"url":"https://codesubmit.io/library/react"}'
# → {"short_url":"http://localhost:8080/0k8s40"}

# Decode (full short URL or bare code both work)
curl -s -X POST localhost:8080/decode \
  -d '{"short_url":"http://localhost:8080/0k8s40"}'
# → {"original_url":"https://codesubmit.io/library/react"}

# Redirect (bonus)
curl -s -i localhost:8080/0k8s40   # → 302 with Location: <original>

# Health
curl -s localhost:8080/health      # → {"status":"ok"}
```

## Tests

```bash
make test          # go test -race ./...
make cover         # coverage summary (internal/* ≥85%)
make cover-html    # write + open an HTML coverage report
make vet           # go vet ./...
```

Or directly:

```bash
go test -race ./...
go test -race -coverprofile=cover.out ./... && go tool cover -func=cover.out
```

## Docker

```bash
docker build -t shortlink .
# Persist the SQLite file on a host volume so data survives container restarts.
docker run --rm -p 8080:8080 \
  -e FEISTEL_KEY=change-me \
  -e DB_PATH=/data/shortlink.db \
  -v "$(pwd)/data:/data" \
  shortlink
```
