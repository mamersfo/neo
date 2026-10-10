# Neo

`neo` is a Supabase-CLI-like tool for the Neo4j graph database: it provisions a
local Neo4j Community container via Docker and manages Cypher migrations from
version-controlled files. Single static binary, no runtime required.

Design background: `design.md` (ADRs), build order: `plan.md`.

## Requirements

- Docker (running daemon; `DOCKER_HOST` respected)
- That's it — `neo` needs no Node/Python/Java runtime

## Install

From a release tarball (recommended):

```sh
curl -L https://github.com/mamersfo/neo/releases/latest/download/neo_0.1.0_Darwin_arm64.tar.gz | tar xz
sudo mv neo /usr/local/bin/neo
# templates live in ./neo/lib next to the binary (optional override, see below)
```

From source:

```sh
go install github.com/mamersfo/neo@latest
# or
git clone https://github.com/mamersfo/neo && cd neo && make build  # → bin/neo
```

## Quickstart

```sh
neo init                        # create neo/neo.toml + migrations/ + seed.cypher
neo start                       # pull neo4j:5.26, start container, print credentials
neo status                      # container state + Bolt/HTTP URLs
neo migration new "create person"
# …edit the new file…
neo db reset                    # wipe + replay migrations + seed
neo db diff                     # dump live schema to a new migration file
neo stop                        # stop (keeps data); --remove to delete
```

## Commands (Supabase equivalents)

| neo | supabase | notes |
|---|---|---|
| `neo init [--dir .] [--force]` | `supabase init` | Local files only; generates random password in `neo.toml` |
| `neo start [--timeout 60] [--pull missing\|always\|never]` | `supabase start` | First run pulls image; password printed **only here** |
| `neo status [--output table\|json]` | `supabase status` | Table hides password; json includes it (for scripts) |
| `neo stop [--timeout 10] [--remove]` | `supabase stop` | Idempotent; `--remove` deletes the container |
| `neo migration new <name>` | `supabase migration new` | `neo/migrations/<UTC-ts>_<slug>.cypher` |
| `neo db reset [--seed] [--timeout 120]` | `supabase db reset` | Wipe → replay → `seed.cypher` (skip with `--seed=false`) |
| `neo db diff [--output file]` | `supabase db diff` | Schema-only dump, see limits |

Exit codes: `0` ok, `1` usage/config, `2` docker, `3` neo4j/cypher.

## Configuration (`neo/neo.toml`)

```toml
[neo]
image = "neo4j:5.26"   # LTS pin; set a 2025.x tag to test CalVer releases
bolt_port = 7687
http_port = 7474
user = "neo4j"
password = "<generated-on-init>"   # local secret, printed once by start

[migrations]
dir = "migrations"
```

Safe to commit except `password` (keep it local or secret-manage it).

Migration files support multiple statements (split client-side; `;` inside
strings/comments is respected). Keep `CREATE CONSTRAINT/INDEX` in data-free
files — one statement runs per transaction. History is recorded in
`_neo_migrations` nodes (filename, checksum, applied_at).

## `neo/lib` and `$NEO_LIB_DIR`

The binary embeds its templates but also honors co-located assets (ADR-002):

1. `$NEO_LIB_DIR` (dev/operator override)
2. `<exe-dir>/lib` (e.g. `/usr/local/bin/lib` next to `neo`)
3. Embedded defaults

Release tarballs ship `neo/lib/neo.toml.tmpl` and `migration.tmpl.cypher` —
reference copies you can customize via (1)/(2). Unit tests pin the embedded
copies equal to these files (drift fails the build).

## `db diff` limitations (v1, by design)

- **Schema only**: constraints + indexes. Graph data is never dumped.
- Ambiguous objects become commented `-- TODO(diff)` lines — review them.
- Lookup indexes are skipped (auto-managed); constraint-backed indexes point
  at `CREATE CONSTRAINT` instead of duplicating.
- No `--from/--to` state comparison yet (v2). Always `neo db reset` after a
  diff to prove the file applies cleanly.

## Development

```sh
make build      # bin/neo (VERSION=… injects --version)
make vet test fmt
go test ./...   # unit (no Docker needed)
# integration (Docker + pulls image, ~1 min):
go test -tags integration ./internal/docker ./internal/migrate
```

See `AGENTS.md` (repo conventions), `design.md` (ADRs), `plan.md` (phases).
