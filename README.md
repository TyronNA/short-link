# ShortLink

A URL shortening service written in Go.

> **Status: Planning phase.** Implementation has not started yet. This commit locks the
> agreed requirements, project rules, and implementation plan before any code is written.

## Planning Documents

| Document | Purpose |
|----------|---------|
| [requirement.md](requirement.md) | Requirements captured from the assignment (R1–R10), deliverables, evaluation criteria, and locked technical decisions |
| [RULES.md](RULES.md) | Project conventions (layout, error handling, API, persistence, testing) |
| [PLAN.md](PLAN.md) | 9-step implementation plan, 3-tier test strategy, and Definition of Done |

## Design Summary (locked)

- **Language:** Go (stdlib `net/http`, minimal dependencies)
- **Storage:** SQLite via `modernc.org/sqlite` (pure-Go, CGO-free) — persists after restart
- **Short code:** auto-increment counter → Feistel bijective permutation → 6-char base62
  (no collisions, non-enumerable, matches the `GeAi9K` example)
- **Endpoints:** `POST /encode`, `POST /decode` (JSON), plus `GET /{code}` redirect

The full README (API reference, security/attack-vector analysis, scalability discussion)
and `RUNNING.md` will be written during the implementation phase (see PLAN.md step 6).
