# Neo Tool — Design Decisions (ADRs)

Date: 2026-10-10 (rev.2 — Go added)
Status: Proposed
Deciders: mamersfo, Muse Spark (AI assistant, pi harness)
Context: `AGENTS.md` — Supabase-CLI-like tool for Neo4j (Community Edition in Docker, `init`/`start`/`status`/`stop`, `migration new` / `db reset` / `db diff`, `seed.cypher`).

This document records the platform trade-off (Rust vs. TypeScript vs. Python, plus Go)
and the downstream design decisions it forces. Format follows MADR-style ADRs.

**Revision history:**
- rev.1 (2026-10-10): Rust vs. TypeScript vs. Python → chose Rust (ADR-001).
- rev.2 (2026-10-10): added **Go (Option D)** at your request. New winner: **Go**.
  ADR-001 is kept for history and marked *Superseded*; the new decision is **ADR-007**.
  ADR-002…006 gain a "Go mapping" amendment so the rest of the doc stays buildable.

---

## Decision drivers (weighted)

Ordered by importance as requested. `Proficiency` is weighted highest (`3x`),
`Maintainability` and `Speed` next (`2x`), rest `1x`.

1. **Agent proficiency (3x)** — most important per request: how reliably *I* (Muse Spark,
   operating in the `pi` agentic edit/compile/test loop) can generate correct,
   working code in each platform with minimal iteration.
2. **Maintainability (2x)** — type safety, refactorability, error handling for a
   long-lived CLI that shells out to Docker and talks Bolt/Cypher.
3. **Speed (2x)** — CLI startup latency + runtime throughput + CI build time.
4. **Ease of use (1x)** — onboarding, toolchain setup, REPL/docs, contributor friction.
5. **Effectiveness (1x)** — conciseness / expressivity for this domain:
   CLI parsing, filesystem migrations, Docker orchestration, Cypher execution.

> Note on "speed": for a CLI, cold-startup (<50ms) and single-binary distribution
> matter more than raw compute. All four platforms are "fast enough" to run
> Cypher; Neo4j + Docker dominate wall-clock time, not the CLI language.

---

## Options considered

- **A: Rust** (your 1st preference) — `cargo`, `clap`, `bollard`, `neo4rs` (community).
- **B: TypeScript (Node.js)** (your 2nd preference) — `commander`/`yargs`, `dockerode`, official `neo4j-driver`.
- **C: Python** (your 3rd preference) — `click`/`typer`, `docker-sdk`, official `neo4j` driver.
- **D: Go** (new) — `cobra`, official `docker/docker` client, official `neo4j-go-driver/v5`.
  This is not a random addition: **Supabase CLI itself is written in Go**, so D is the
  exact-stack parity option.

### Scorecard (1 = poor, 5 = excellent)

| Criterion (weight)                | Rust (A) | TypeScript (B) | Python (C) | **Go (D)** | Notes |
|-----------------------------------|----------|----------------|------------|------------|-------|
| **Proficiency — raw familiarity** | 4 | 5 | 5 | **5** | LLM training volume: Go ≈ Py > TS > Rust. Go's small spec means I rarely hallucinate APIs. |
| **Proficiency — agentic verifiability** | 5 | 3 | 2 | **4** | `rustc`+`clippy` strictest; `go build`+`go vet`+`gofmt` close second and *much faster*; `tsc` partial; Py runtime-only. |
| **Proficiency — weighted total**  | ~4.7 | ~4 | ~3.5 | **~4.7** | Tie Rust/Go on the highest-weighted driver; see appendix. |
| Maintainability (2x)              | 5 | 4 | 2 | **4** | Rust's `Result`/exhaustive match is best; Go's `gofmt`+static types+simplicity is proven at CLI scale (Docker, K8s, Supabase CLI). Py weakest. |
| Speed — CLI startup/runtime (2x)  | 5 | 3 | 3 | **5** | Rust ~5ms, Go ~10ms native; Node ~80ms + `node_modules`; Python ~30ms + interpreter. Throughput irrelevant vs. Docker pull. |
| Speed — dev iteration             | 2 | 4 | 5 | **4** | `cargo build` slowest (borrow-checker fights); `go build` cached in seconds; Py/TS instant. Not separately weighted but breaks the Rust/Go tie in practice. |
| Ease of use (1x)                  | 3 | 4 | 5 | **4** | Rust borrow checker + async colors steepest; Go single toolchain, no async colors, `go run`. Py easiest (REPL). Note: `go` isn't on this machine yet (`go: command not found`) — one `brew install go` fixes it. |
| Effectiveness — conciseness (1x)  | 3 | 4 | 5 | **3** | Py tersest; Go verbose (`if err != nil`) but stdlib `os/exec/path/filepath` + `cobra` + Docker SDK is *pragmatically* ideal for this domain. Tie with Rust, different shape. |
| Distribution (`neo/lib` req.)     | 5 | 2 | 2 | **5** | Rust + Go: single static binary (`CGO_ENABLED=0`, `GOOS/GOARCH` cross-compile trivial) + sidecar `lib/`. TS/Py need runtimes. |
| Ecosystem for this domain         | 3 | 5 | 5 | **5** | Only B/C/D have **official** Neo4j drivers; Rust `neo4rs` is community. Docker: Go uses the *reference* client (`docker/docker`); `bollard`/`dockerode`/`docker-sdk` are ports. |
| Alignment with Supabase CLI       | 4 | 2 | 2 | **5+** | Supabase CLI **is** Go + Cobra. With Go we port patterns/file layouts/tests directly instead of translating them. Rust was only "closest analogue"; Go is identical. |

**Weighted winner (rev.2): Go (D), narrowly over Rust (A).**
Go ties Rust on proficiency (highest weight), speed, and distribution; beats it on
ecosystem, Supabase alignment, dev-iteration speed, and ease of use; loses only on
raw type-system strictness (maintainability 4 vs. 5) and conciseness nuance.
TypeScript/Python lose on distribution + verifiability regardless.

---

## ADR-001: Implementation platform — Rust [SUPERSEDED]

- **Status:** ~~Accepted~~ → **Superseded by ADR-007 (rev.2).**
- **Context (rev.1):** Need to pick one of Rust / TypeScript / Python for `neo`.
- **Decision (rev.1, preserved for history):** Implement in Rust (`clap`, `tokio`, `bollard`, `neo4rs`).
- **Why superseded:** rev.1 explicitly excluded Go. Once Go is in the mix, Go matches Rust
  on every deciding factor (compiler-verified agent output, single binary, maintainability)
  while fixing Rust's two biggest weaknesses: (1) community-only Neo4j/Docker drivers,
  (2) slow agentic iteration (borrow checker + long builds). See ADR-007.
- **Alternatives (rev.1):** TypeScript / Python — rejected, reasoning unchanged.

### Proficiency appendix (rev.1, preserved)

As Muse Spark I am proficient in all three (Py ≈ TS > Rust on raw recall).
But in the `pi` harness what matters is **closed-loop reliability**, not recall:
`cargo build` + `clippy` reject hallucinations fast; `tsc` partial; Python runtime-only.
Hence effective delivery Rust > TS > Py.

---

## ADR-002: Distribution — single binary + co-located `lib/`

- **Status:** Accepted (wording amended by ADR-007).
- **Context:** `AGENTS.md`: "easy to install the tool somewhere on the local file system ...
  extra files ... co-located with the executable, e.g. within a `neo/lib` folder."
- **Decision (rev.1, Rust wording):** one binary; support files relative to `current_exe()/lib/`,
  `NEO_LIB_DIR` override, GH Releases + Homebrew, host Docker socket required.
- **Go mapping (amendment per ADR-007):**
  - Resolve `lib/` via `os.Executable()` + `filepath.Join(dir, "lib")`; `NEO_LIB_DIR` override unchanged.
  - Build with `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` (~8–15MB binary).
  - Release via **GoReleaser** (GH Releases for darwin/linux arm64/amd64) + Homebrew tap;
    `go install github.com/<org>/neo@latest` as dev fallback. No `cargo install` / musl matrix needed —
    Go cross-compiles from one runner.
- **Consequences:** unchanged — `curl | tar xz` install, no runtime to manage.
- **Alternatives:** npm global / PyPI — rejected per ADR-001/007.

---

## ADR-003: CLI surface — mirror Supabase CLI structure

- **Status:** Accepted (wording amended by ADR-007).
- **Context:** Required parity: `init`, `start`, `status`, `stop`, `migration new`, `db reset`, `db diff`.
- **Decision:**
  ```
  neo init [--dir .]
  neo start [--detach]
  neo status [--output json|table]
  neo stop [--timeout 10]
  neo migration new <name>      # creates neo/migrations/<UTC-timestamp>_<slug>.cypher
  neo db reset [--seed/--no-seed]
  neo db diff [--from migration|empty] [--to live] [--output file]
  neo --help / --version, per-command --help, shell completions
  ```
  - Rev.1: `clap` derive, `src/cmd/` modules. **Rev.2 (Go): `spf13/cobra` + `spf13/viper`
    (flags/config), one package per subcommand under `cmd/`** (`cmd/init.go`, `cmd/start.go`, …),
    `internal/` for docker/neo/config logic. This mirrors Supabase CLI's own layout, so its
    source becomes a copy-paste reference.
  - Exit codes: `0` ok, `1` usage/config error, `2` Docker error, `3` Neo4j/Cypher error (unchanged).
  - `neo status --output json` for scripts; Cobra generates help + completions (`neo completion bash|zsh|fish`).
- **Consequences:** Supabase users feel at home; completions free either way; Go version lets us
  lift Cobra patterns directly from Supabase CLI.
- **Alternatives:** hand-rolled/`lexopt` (Rust) / `urfave/cli` (Go) — rejected (more custom code).

---

## ADR-004: Project layout + config — `neo/` directory, `neo.toml`

- **Status:** Accepted (unchanged, Go library swapped).
- **Context:** `neo init` must create committable local files only, no AuraDB linkage.
- **Decision:**
  ```
  ./neo/
    neo.toml               # ports, image tag, auth, migration dir (committed)
    migrations/
      20261010120000_create_person.cypher
    seed.cypher            # optional, runs after reset (committed)
  ```
  - TOML shape unchanged: `[neo] image="neo4j:5-community" bolt_port=7687 http_port=7474 user="neo4j"`, `[migrations] dir="migrations"`.
  - Rev.1: `serde` + `toml` crate. **Rev.2 (Go): `BurntSushi/toml` or `viper` (TOML)** — same file, same fields.
  - `init` idempotent; refuses overwrite without `--force`; project root found by walking up (like `git`).
- **Consequences:** Safe to commit; deterministic `start`/`reset`.
- **Alternatives:** YAML — rejected (TOML matches `cargo`- *and* Go-config conventions, single file).

---

## ADR-005: Local runtime — Docker, Neo4j Community image

- **Status:** Accepted (wording amended by ADR-007).
- **Context:** `start`/`stop`/`status` must provision local Neo4j; first run pulls image.
- **Decision:**
  - Behavior unchanged: pull pinned `neo4j:5-community` if missing, container `neo-<project-hash>`
    with `NEO4J_AUTH`, Bolt+HTTP maps, `NEO4J_PLUGINS=["apoc"]`, healthcheck on `:7474`,
    stream pull progress, poll Bolt until ready (60s timeout); `stop` stops (not removes) by
    label `managed-by=neo`; `status` prints state + URLs + credentials discipline as before.
  - Rev.1: `bollard` crate. **Rev.2 (Go): official `github.com/docker/docker/client`** (the reference
    implementation — no port lag, every Supabase/Docker-CLI example applies verbatim). Fallback to
    shelling `docker` CLI on socket errors unchanged.
- **Consequences:** No `docker-compose.yml`; with Go, Docker examples/SDK docs apply 1:1.
- **Alternatives:** `docker-compose`/`testcontainers-go` — compose rejected (extra file);
  `testcontainers-go` reserved for integration tests only.

---

## ADR-006: Migrations + connectivity — ordered `.cypher` files over Bolt, APOC-assisted diff

- **Status:** Accepted (wording amended by ADR-007).
- **Context:** Need `migration new`, `db reset` (replay + seed), `db diff` for a schema-optional graph
  (labels/relationships/constraints/indexes — no tables, no canonical SQL-style DDL diff).
- **Decision:**
  - `migration new <name>`: UTC timestamp prefix, slugified name, `-- up` / `-- down` markers (down optional v1). Unchanged.
  - History: `_neo_migrations(applied_at, filename, checksum)` nodes; `db reset` wipes + replays in
    lexicographic order in transactions + runs `seed.cypher`. Unchanged.
  - Rev.1: `NeoClient` trait over `neo4rs`. **Rev.2 (Go): `NeoClient` interface over
    official `github.com/neo4j/neo4j-go-driver/v5`** (`ExecuteQuery` / managed transactions);
    health check via Bolt handshake. Driver swap stays one file, but starts from the supported driver.
  - `db diff` v1 scope unchanged (honest stub): `SHOW CONSTRAINTS` / `SHOW INDEXES` /
    `apoc.meta.schema()` live vs. last-applied snapshot → new timestamped `.cypher` with `// TODO`
    markers. Full semantic graph-diff out of scope v1.
- **Consequences:** Deterministic `reset`; useful-but-honest `diff`.
- **Alternatives:** `liquibase`/`neo4j-migrations` JVM sidecar in `lib/` — deferred to v2, allowed by ADR-002.

---

## ADR-007: Implementation platform (revisited, with Go) — Go

- **Status:** Accepted (supersedes ADR-001).
- **Context:** ADR-001 chose Rust from {Rust, TS, Python}. You pointed out the omission I
  flagged myself: Supabase CLI is Go, so any "Supabase-CLI-like" trade-off without Go is
  incomplete. Re-running the analysis with D: Go.
- **Decision:** Implement `neo` in **Go (1.23+, `go.mod`, `gofmt`/`go vet` clean)**.
  - CLI/config: `spf13/cobra` + `spf13/viper` (or `BurntSushi/toml` if Viper feels heavy).
  - Docker: official `github.com/docker/docker/client`.
  - Neo4j: official `github.com/neo4j/neo4j-go-driver/v5`.
  - Logging: stdlib `log/slog` (JSON in `--output json` paths, human otherwise).
  - Errors: `fmt.Errorf("...: %w", err)` + `errors.Is/As`; exit-code mapping per ADR-003.
  - Layout: `cmd/` (one file per subcommand), `internal/config`, `internal/docker`,
    `internal/neo`, `internal/migrate`; `neo/lib/` runtime assets; `go.mod`/`go.sum` committed.
- **Consequences:**
  - *Positive:* single static binary + `lib/` (ADR-002) with trivial cross-compile; official
    Docker + Neo4j drivers (no community-driver risk); Supabase CLI source becomes a direct
    template (flags, output shapes, pull-progress, error strings); `go build`/`go test` loop is
    seconds, keeping the `pi` agent loop tight; `gofmt` ends style debates.
  - *Negative:* weaker type system than Rust (no exhaustive enums, `nil` maps/slices bite;
    mitigate with `vet`, `staticcheck`, table tests); verbose error handling (`if err != nil`
    everywhere — mitigate with small helpers); marginally larger binaries than stripped Rust;
    Go toolchain not yet on this dev machine (one `brew install go`).
  - *Neutral:* contributors need Go, not Cargo — equally one install; pin via `go.mod` + CI image.
- **Alternatives considered:**
  - *Rust (ADR-001, now runner-up):* keep if you value maximal type safety / your stated
    preference order over ecosystem parity. Costs: community `neo4rs`, `bollard` port lag,
    slow builds + borrow-checker churn in the agent loop, and translating (not porting)
    Supabase CLI logic. If Go is vetoed for preference reasons, fall back to ADR-001 unchanged.
  - *TypeScript:* rejected — official driver but Node-runtime distribution violates ADR-002;
    `tsc` verifies less than `go vet`; `node_modules` weight.
  - *Python:* rejected — tersest and my best raw recall, but runtime-only verification (slowest
    agent loop), weakest maintainability, interpreter-distribution drift.

### Proficiency appendix (rev.2 — why Go ties/wins on the most important criterion)

Raw recall: Py ≈ Go ≈ TS > Rust (Go's tiny spec + huge corpus = few hallucinations).
Closed-loop verifiability: Rust 5 > Go 4 > TS 3 > Py 2 — but loop *cost* inverts:
Rust failures often need borrow-checker redesign (minutes), Go failures are usually a
wrong field/missing `err` check fixed from `go vet` output (seconds). In the `pi`
harness, **throughput-corrected delivery is Go ≥ Rust > TS > Py**, which at 3x weight
is decisive combined with Go's official-driver + Supabase-parity advantages.

---

## Roadmap implied by these ADRs (rev.2, Go)

1. `go mod init github.com/<org>/neo`, add `cobra`, `docker/docker/client`, `neo4j-go-driver/v5`, TOML lib, `slog`.
2. Scaffold `cmd/{init,start,status,stop,migration,db}.go` + `internal/{config,docker,neo,migrate}` (ADR-003/007) — steal layout from Supabase CLI.
3. Implement `init` + `neo.toml` (ADR-004) — testable without Docker (`go test ./...`).
4. Implement `start`/`stop`/`status` via official Docker client (ADR-005) — integration test with Docker.
5. Implement `NeoClient` interface + `migration new` + `db reset` + `seed.cypher` (ADR-006).
6. Implement `db diff` v1 stub (ADR-006) + `status --output json`.
7. Release plumbing (ADR-002): GoReleaser + GH Actions (darwin/linux × arm64/amd64) + Homebrew tap.
8. One-time: `brew install go` on this machine (currently `go: command not found`; Rust/Node/Python already present).

---

## References

- `AGENTS.md` (this repo) — command parity + `neo/lib` requirement.
- Supabase local dev: https://supabase.com/docs/guides/local-development ; DB migrations: https://supabase.com/docs/guides/local-development/database-migrations ; CLI access/credentials: https://supabase.com/docs/guides/local-development/cli/getting-started#access-your-projects-services
- Supabase CLI source (Go + Cobra — direct template for ADR-007): https://github.com/supabase/cli
- Go libs (verify via Context7 during implementation): `spf13/cobra`, `spf13/viper`, `docker/docker/client`, `neo4j/neo4j-go-driver/v5`, `BurntSushi/toml`, stdlib `log/slog`.
- Rust alternatives if ADR-001 fallback invoked: `clap`, `tokio`, `bollard`, `neo4rs`.
