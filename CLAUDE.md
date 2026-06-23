# ShortLink — Project Guide for Claude

A URL shortening service in Go. This file auto-loads into every session. Read it first,
then read the three planning docs before writing any code.

## Read these first (source of truth — do not contradict them)

1. **[requirement.md](requirement.md)** — assignment requirements (R1–R10), deliverables,
   evaluation criteria, and **locked technical decisions** (§7). These decisions are final.
2. **[RULES.md](RULES.md)** — project conventions you MUST follow (layout, error handling,
   API shape, persistence, testing, naming, git).
3. **[PLAN.md](PLAN.md)** — the 9-step implementation plan, 3-tier test strategy, and
   Definition of Done. **Implement strictly in this order.**

## Locked design (do not re-litigate without the user's explicit OK)

- **Language/router:** Go, `net/http` stdlib only (`ServeMux` method patterns, Go 1.22+).
  Minimal third-party deps.
- **Storage:** SQLite via `modernc.org/sqlite` (pure-Go, CGO-free). Must persist after
  restart (requirement R6). Parameterized queries only — never string-concat SQL.
- **Short code:** auto-increment counter (SQLite `AUTOINCREMENT`) → **Feistel bijective
  permutation** over `[0, 62^6)` → **6-char base62**. This gives: no collisions (bijection),
  non-enumerable codes (Feistel mixing), matches the `GeAi9K` example, decode = inverse
  (no reverse lookup table needed). The DB only stores `id ↔ url`.
- **Idempotency:** UNIQUE index on `url` so the same URL always maps to the same code.
- **API:** `POST /encode` (201), `POST /decode` (200/404), both JSON. Bonus: `GET /{code}`
  302 redirect, `GET /health`.

## Package layout

```
cmd/server/main.go        — wiring + config (env: PORT, DB_PATH, BASE_URL, FEISTEL_KEY) + graceful shutdown
internal/shortener/       — Feistel + base62 (code <-> id), pure functions, no I/O
internal/store/           — SQLite data access (id <-> url), nothing HTTP-aware
internal/handler/         — HTTP handlers, JSON, input validation; depends on a Store interface (for mocking)
```

Dependency rule: `store` and `shortener` must NOT import `handler`. Handler depends on a
consumer-side `Store` interface, not a concrete type.

## Workflow rules

- Implement PLAN.md steps **in order**; each step must `go build ./...` and `go test ./...`
  green before moving on.
- Security hardening is part of the build, not an afterthought: body-size limit
  (`http.MaxBytesReader`), validate decode code (length 6 + base62 charset) before
  deobfuscating, `X-Content-Type-Options: nosniff`, Feistel key from env. See the security
  audit table in the conversation / to be written into README §Security.
- Tests: unit (3 packages, table-driven) + **e2e** (real httptest server + real SQLite on
  `t.TempDir()`, including a restart-and-decode test for R6). Target ≥85% coverage on
  `internal/*`. Run `go test -race ./...`.
- Docs (README full version + RUNNING.md) are PLAN.md step 6 — describe the Feistel design,
  NOT hash collisions.
- Git: commit in small logical chunks with `feat:`/`test:`/`docs:` prefixes. **Do NOT push
  or commit unless the user asks.** Remote is `git@github.com:TyronNA/short-link.git`.

## Current status

**Implementation complete (PLAN.md Steps 1–6).** Code lives in `cmd/server` and
`internal/{shortener,store,handler,e2e}` exactly as designed (SQLite + Feistel → base62(6)).
`go build`, `go vet`, and `go test -race ./...` are all green; `internal/*` coverage ≥85%.
README/RUNNING/Dockerfile/Makefile/.env.example are written.

Remaining: **Step 7** push to GitHub (commits are on `master`, not yet pushed — remote URL
needs confirming), **Step 8** deploy to a free server, **Step 9** draft the submission email.
