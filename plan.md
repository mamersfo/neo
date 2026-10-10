# Neo Build Plan (Go)

Date: 2026-10-10
Source: `AGENTS.md` + `design.md` ADR-002…007
Toolchain verified: `go1.27.2 darwin/arm64`, Docker `29.8.2`. No `go.mod` yet — Phase 0 creates it.

Goal: Supabase-CLI-like `neo` binary in Go: `init` / `start` / `status` / `stop` / `migration new` / `db reset` / `db diff`, local Neo4j Community in Docker, file-based `.cypher` migrations + `seed.cypher`.

---

## 0. Target layout (to be created)

```
neo/
  main.go                    # cobra root, exit-code mapping
  go.mod / go.sum
  cmd/
    root.go                  # root cmd, --version, completion
    init.go                  # neo init
    start.go / stop.go / status.go
    migration.go             # neo migration new
    db.go                    # neo db reset | diff
  internal/
    config/config.go         # find root, load/save neo/neo.toml
    docker/docker.go         # client, pull, create/start/stop/inspect
    neo/client.go            # NeoClient interface + go-driver impl, wait-ready
    migrate/migrate.go       # list, new, checksum, replay, history
  neo/lib/                   # runtime assets (templates, helpers) — resolved via os.Executable()+NEO_LIB_DIR
    neo.toml.tmpl
    migration.tmpl.cypher
  plan.md / design.md / README.md
```

Module path (decided 2026-10-10): `github.com/mamersfo/neo`.
Pinned deps (Phase 0, `go get`):
- `github.com/spf13/cobra@latest`, `github.com/spf13/viper@latest` (decided — viper, no BurntSushi)
- `github.com/docker/docker@latest` (must support Docker 29.x client)
- `github.com/neo4j/neo4j-go-driver/v5@latest`
- stdlib `log/slog` for logging, `encoding/json` for `--output json`.
- Default image (decided — LTS): `neo4j:5.26` (exact pin, Community = no suffix). Override via `neo.toml`.

Exit codes: `0` ok, `1` usage/config, `2` docker, `3` neo4j/cypher.

---

## Phase 0 — Bootstrap (0.5 day)

1. `go mod init github.com/mamersfo/neo`, `go get` deps above, `go mod tidy`.
2. Scaffold `main.go` + `cmd/root.go` (cobra root, `--version`, `completion` cmd), `go build -o /tmp/neo .` works.
3. Add `Makefile` targets: `build`, `vet`, `test`, `lint` (`go vet ./...` + `gofmt -l .`; `staticcheck` optional).
4. TOML lib: decided — `viper` (per 2026-10-10 decisions).
5. **Done when:** `go build ./... && go vet ./... && /tmp/neo --help` prints stub commands.

## Phase 1 — `neo init` + config (1 day, no Docker needed)

Files: `internal/config/config.go`, `cmd/init.go`, `neo/lib/neo.toml.tmpl`.
- `FindRoot()`: walk up from cwd looking for `neo/neo.toml` (like git).
- `Init(dir)`: idempotent; `mkdir neo/migrations`, write `neo.toml` (image `neo4j:5.26` LTS pin, bolt `7687`, http `7474`, user `neo4j`, generated password — decided 2026-10-10, stored in `neo.toml`, never committed secrets without warning), write `seed.cypher` placeholder + `.gitkeep`; `--force` overwrites.
- Unit tests: init in `t.TempDir()`, re-init without `--force` errors, `--force` ok, TOML round-trips.
- **Done when:** `neo init && cat neo/neo.toml && neo init` (2nd fails cleanly) passes; `go test ./internal/config -v` green.

## Phase 2 — Runtime: `start` / `stop` / `status` (2–3 days)

Files: `internal/docker/docker.go`, `internal/neo/client.go`, `cmd/start.go|stop.go|status.go`.
- Docker client from env (`DOCKER_HOST` / `~/.docker`), require socket (clear error if missing).
- `start`: pull pinned image w/ progress → create-or-start container `neo-<short-hash-of-root>` labeled `managed-by=neo`, env `NEO4J_AUTH`, ports Bolt/HTTP, `NEO4J_PLUGINS=["apoc"]` → poll Bolt via driver handshake ≤60s → print Bolt URL + creds (password only here).
- `stop [--timeout] [--remove]`: stop by label/name; `--remove` also `rm`.
- `status [--output table|json]`: container state + mapped ports + Bolt/HTTP URLs; json for scripts.
- Tests: unit with fake Docker interface; integration (build-tag `integration`, needs Docker): start→status→stop cycle against real daemon.
- **Done when:** fresh `neo start` (cold pull) → `neo status` → `neo stop` → `neo status` (stopped) all green; `docker ps --filter label=managed-by=neo` agrees.

## Phase 3 — Migrations: `migration new` + `db reset` + seed (2–3 days)

Files: `internal/migrate/migrate.go`, `cmd/migration.go`, `cmd/db.go`, `neo/lib/migration.tmpl.cypher`.
- `migration new <name>`: slugify, UTC `%Y%m%d%H%M%S` prefix, write `neo/migrations/<ts>_<slug>.cypher` with `-- up`/`-- down` markers.
- `NeoClient` interface (`ExecuteBatch`, `Query`, `Close`) over official v5 driver; all Cypher behind it.
- History nodes `_neo_migrations(filename, checksum, applied_at)`; `db reset [--seed/--no-seed]`: wipe (`DETACH DELETE` + drop constraints/indexes in order) → replay files lexicographically in transactions → record history → run `seed.cypher` unless `--no-seed` / missing.
- Checksum (sha256) to detect edited migrations.
- Tests: unit (ordering, slugify, checksum, template); integration: reset on live DB replays 2 sample migrations + seed, history nodes correct, re-reset idempotent.
- **Done when:** `neo migration new create_person` creates file; `neo db reset` on running DB loads data; `MATCH (n) RETURN count` reflects migrations+seed.

## Phase 4 — `db diff` v1 honest stub (1–2 days)

File: `internal/migrate/diff.go` (+ `cmd/db.go` subcommand).
- Scope (per ADR-006): dump live `SHOW CONSTRAINTS` / `SHOW INDEXES` / `apoc.meta.schema()` vs last-applied snapshot → emit new timestamped `.cypher` with concrete `CREATE CONSTRAINT/INDEX` where unambiguous + `// TODO` markers where semantic (labels/rel-types/property presence can't be derived). Document limitation in `--help` + README.
- **Done when:** after manual graph change, `neo db diff --output neo/migrations/<ts>_diff.cypher` produces appliable-or-clearly-marked file; no silent data loss.

## Phase 5 — Packaging, docs, CI (1 day)

- `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`, `lib/` resolution via `os.Executable()` + `NEO_LIB_DIR` override.
- GoReleaser + GH Actions (darwin/linux × arm64/amd64) + Homebrew tap; `go install` fallback.
- README: install, each command with Supabase equivalent, `neo/lib` note, diff limitations.
- `gofmt`, `go vet`, full `go test ./...`, cold-install smoke test (`curl|tar` → `neo --help`).
- **Done when:** release artifact installs on clean machine and passes Phase 2–3 smoke.

---

## Test strategy

- `go test ./...` always green without Docker (integration behind `-tags integration`, skipped otherwise).
- Phase 1–3 each add unit tests; Phase 2–3 add one integration test each (real Docker/Neo4j).
- Manual smoke script (also CI job once stable): `init → start → status → migration new → db reset → diff → stop`.

## Risks / open questions (all decided 2026-10-10)

1. **Neo4j image tag:** decided — LTS `neo4j:5.26` exact pin. Phase 2 verifies `NEO4J_PLUGINS` path; `2025.x` via `neo.toml` override + CI matrix only.
2. **Driver API drift:** pin `neo4j-go-driver/v5` version in Phase 0; consult Context7 for `ExecuteQuery` vs session patterns during Phase 3.
3. **Docker client vs daemon version (29.8.2):** pin `docker/docker` to a 29.x-compatible release; fallback to `docker` CLI on socket errors.
4. **Port conflicts (7687/7474):** Phase 2 should detect occupied ports and error with override hint (`neo.toml` edit).
5. **Password handling:** decided — generate random password on `init`, store in `neo.toml`, print once on `start` (Supabase-like).
6. **Module path:** decided — `github.com/mamersfo/neo`.

## Proposed execution order

Phase 0 → Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5. Phases 1–3 demoable independently; do not start Phase 4 until `db reset` integration is green.
