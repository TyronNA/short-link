# ShortLink

A URL shortening service written in Go. It maps a long URL to a fixed 6-character
short code and back, persists across restarts, and exposes a small JSON API.

```
POST /encode  {"url":"https://codesubmit.io/library/react"}  →  {"short_url":"http://host/0k8s40"}
POST /decode  {"short_url":"http://host/0k8s40"}             →  {"original_url":"https://codesubmit.io/library/react"}
```

See **[RUNNING.md](RUNNING.md)** for how to build, run, test, and use it.

## Design

| Concern | Choice |
|---------|--------|
| Language / router | Go, `net/http` stdlib only (`ServeMux` method patterns, Go 1.22+) |
| Storage | SQLite via `modernc.org/sqlite` (pure-Go, CGO-free) — single file, persists across restarts |
| Short code | auto-increment ID → **Feistel bijective permutation** over `[0, 62⁶)` → **6-char base62** |
| Idempotency | `UNIQUE` index on `url`; same URL always maps to the same code |

### How a code is generated

```
            INSERT url            Feistel PRP            base62 (pad 6)
  url  ───────────────────►  id  ───────────────►  x  ───────────────►  "0k8s40"
        (SQLite AUTOINCREMENT)   (keyed, bijective)     (fixed length)

  decode is the exact inverse: base62 → x → Feistel⁻¹ → id → SELECT url
```

The ID is monotonically increasing, but a counter rendered straight to base62 would be
trivially enumerable (`...001`, `...002`, …). To prevent that, the ID is run through a
**Feistel network** — a keyed pseudo-random permutation — before being rendered. Because a
Feistel network is a *bijection*, this gives three properties at once:

- **No collisions.** Every ID maps to exactly one code and vice-versa (it is one-to-one by
  construction — not a hash that *might* collide).
- **Non-enumerable.** Adjacent IDs produce codes that differ across multiple positions, so
  you cannot walk the keyspace by incrementing a code.
- **No reverse table.** Decoding is the mathematical inverse of encoding, so the database
  only needs to store `id ↔ url`; the code itself is computed, never looked up.

The domain `62⁶ ≈ 56.8 billion` is not a power of two, so the Feistel runs over the smallest
enclosing power-of-two domain (`2³⁶`) with **cycle-walking**: re-apply the permutation until
the result falls back inside `[0, 62⁶)`. Cycle-walking preserves bijectivity. Implementation:
[internal/shortener/shortener.go](internal/shortener/shortener.go).

### Package layout

```
cmd/server/main.go     wiring + config (env) + graceful shutdown
internal/shortener/    Feistel + base62 (code ↔ id) — pure functions, no I/O
internal/store/        SQLite data access (id ↔ url) — no HTTP awareness
internal/handler/      HTTP handlers, JSON, validation; depends on a Store interface
internal/e2e/          full-stack tests (real server + real SQLite, incl. restart)
```

`store` and `shortener` never import `handler`. The handler depends on a consumer-side
`Store` interface, so it is unit-tested against a mock.

## API

All endpoints accept and return `application/json`. Every response carries
`X-Content-Type-Options: nosniff`.

### `POST /encode`

Request: `{"url": "<http/https URL, ≤2048 chars>"}`

| Status | Meaning |
|--------|---------|
| `201 Created` | `{"short_url": "<base>/<code>"}` |
| `400 Bad Request` | empty/missing URL, non-http(s) scheme, no host, too long, control characters, malformed JSON, unknown fields |
| `413 Payload Too Large` | body exceeds 16 KiB |
| `500` | storage/codec failure (no internal detail leaked) |

Encoding the same URL twice returns the same short URL (idempotent).

### `POST /decode`

Request: `{"short_url": "<full short URL or bare code>"}`

| Status | Meaning |
|--------|---------|
| `200 OK` | `{"original_url": "<url>"}` |
| `400 Bad Request` | code is not exactly 6 base62 characters, malformed JSON |
| `404 Not Found` | well-formed code with no stored URL |

### `GET /{code}` (bonus)

`302 Found` redirect to the original URL, or `400`/`404` as above.

### `GET /health`

`200 OK` `{"status":"ok"}`.

## Security — attack vectors considered

| Vector | Mitigation | Where |
|--------|-----------|-------|
| **SQL injection** | All queries are parameterized; no string concatenation. Verified by a test that stores `'); DROP TABLE links;--`. | [store.go](internal/store/store.go), [store_test.go](internal/store/store_test.go) |
| **DoS via large body** | `http.MaxBytesReader` caps bodies at 16 KiB → `413`. | [handler.go](internal/handler/handler.go) |
| **DoS via slow/hung clients** | Server has `ReadTimeout`, `ReadHeaderTimeout`, `WriteTimeout`, `IdleTimeout`. | [main.go](cmd/server/main.go) |
| **Oversized / junk URLs** | URL length capped at 2048; scheme restricted to `http`/`https`; host required. | `validateURL` |
| **Header / log injection (CRLF)** | URLs containing control characters (incl. `\r`, `\n`) are rejected before storage or logging. | `validateURL` |
| **Code enumeration / scraping** | Feistel permutation makes codes non-sequential; you cannot guess neighbours from one code. | [shortener.go](internal/shortener/shortener.go) |
| **Unvalidated input into codec** | Decode validates length (6) + base62 charset *before* running the inverse permutation. | `Decode`, `resolve` |
| **MIME sniffing** | `X-Content-Type-Options: nosniff` on every response. | `writeJSON` |
| **Internal error disclosure** | 5xx responses return a generic message; details are logged server-side only. | `writeError` |
| **Secret in code** | Feistel key comes from the `FEISTEL_KEY` env var, not source. | [main.go](cmd/server/main.go) |

**Known/accepted:** like every URL shortener, `/decode` and `GET /{code}` are **open
redirects** by design — that is the product's purpose. A production deployment would add a
safe-browsing/host allow-list and rate limiting (out of scope here; see Scalability).

## Scalability & the "collision problem"

**Collisions are eliminated, not patched.** Because codes come from a bijection over
`[0, 62⁶)`, two distinct IDs can never share a code — there is no birthday-paradox risk to
mitigate, unlike a hash-truncation scheme. The same-URL-same-code property is handled
separately by the `UNIQUE(url)` index.

The real scaling questions are:

1. **Distributed unique ID generation.** The design leans on SQLite's `AUTOINCREMENT`, which
   is single-writer. To scale writes horizontally, replace it with a distributed ID source —
   DB sequence range-allocation (hand each node a block of IDs), a Snowflake-style generator,
   or `Redis INCR`. The Feistel/base62 layer is unchanged: it only needs a unique integer.

2. **Keyspace exhaustion.** `62⁶ ≈ 56.8 billion` codes. Approaching that limit, bump the code
   length to 7 (`62⁷ ≈ 3.5 trillion`) — widen the Feistel domain and the pad length. Existing
   6-char codes keep decoding.

3. **Read scaling.** Decoding is pure computation plus one indexed primary-key lookup. Reads
   scale with read replicas / a cache (e.g. Redis) in front of the store; the hot path is the
   redirect.

4. **Storage.** SQLite is ideal for a single-node demo. At scale, move `id ↔ url` to a
   horizontally-partitioned store (sharded by ID range or a managed KV/SQL service); the
   schema is intentionally trivial to migrate.

## Testing

```
go test -race ./...
make cover            # prints coverage; internal/* targets ≥85%
```

Three tiers: unit tests per package (table-driven), and a full-stack `internal/e2e` suite
that runs a real `httptest` server against real SQLite on a temp file — including encoding,
round-trip decode, redirect, idempotency, a 404 path, and **decode-after-restart (R6)**.
