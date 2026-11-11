# AGENTS.md

Guidance for developers and AI agents working in this repository. Keep this
file and `SPEC.md` in **English** — this is a project rule.

## Project overview

`anycastd` is a Go daemon that announces / withdraws anycast (virtual) address
prefixes over BGP based on service health checks. The embedded BGP engine is
GoBGP v3. See `SPEC.md` for the full architecture.

* Module path: `github.com/runityru/anycastd`
* Go version: `1.27.2` (CI uses `1.27.x`)
* The `Makefile` is the single interface for build/test/lint/container tasks.

## Repository layout

```text
cmd/anycastd/           entry point (env, config load, wiring, GoBGP server, services)
config/                 YAML/JSON configuration model + validation (ozzo)
announcer/              Announcer / GoBGPServer interfaces, GoBGP metrics poller
announcer/gobgpv3/      GoBGP v3 adapter (implements GoBGPServer)
checkers/               checker interface + registry; each checker is a subpackage
service/                orchestration: check loop, strategies, core metrics
```

## Build, test, lint

Use the `Makefile` (single interface, modeled on mcp-netbox) for all common
tasks:

```sh
make build             # build the binary
make test              # run all unit tests
make lint              # run golangci-lint + gosec + govulncheck + arch checks
make container-image   # build the multi-arch (amd64+arm64) container image
```

`make lint` runs the sub-targets `lint-golangci`, `lint-gosec`, `lint-vuln`,
and `arch`. CI runs the same Makefile targets, so keep them in sync when
adding/removing targets.

## How checkers work

* A checker implements the `checkers.Checker` interface:

  ```go
  type Checker interface {
      Kind() string
      Check(ctx context.Context) error
  }
  ```

* Checkers are registered into a package-level registry
  (`map[string]func(json.RawMessage) (Checker, error)`, guarded by an
  `RWMutex`) via `checkers.Register` / `checkers.MustRegister`, normally in the
  checker package's `init()`.
* Construction goes through `checkers.NewCheckerByKind(kind, specJSON)`.
* To add a **new checker**:
  1. Create a package under `checkers/<name>/` (e.g. `checkers/mycheck/`).
  2. Define a `spec` struct with JSON tags and an `ozzo` `Validate()` method.
  3. Implement `Kind()` (a `const checkName = "<kind>"`) and `Check(ctx)`.
  4. Provide `NewFromSpec(in json.RawMessage) (checkers.Checker, error)`.
  5. Call `checkers.MustRegister(checkName, NewFromSpec)` in `init()`.
  6. Register any checker metrics with `prometheus.MustRegister(...)` in
     `init()`, using namespace `anycastd`.
  7. Blank-import the new package in `cmd/anycastd/main.go`
     (`_ "github.com/runityru/anycastd/checkers/mycheck"`).
  8. Add a YAML example to `README.md` and document the checker + its metrics
     in `README.md` and `SPEC.md`.

## Mocks and testing

* Interfaces are mocked with **hand-written `testify/mock` mocks**. There is
  **no `mockgen`** and no generated mocks.
* Examples: `checkers/mock.go`, `announcer/mock.go`, `announcer/gobgp_mock.go`.
* The GoBGP adapter is tested against a real in-memory `server.BgpServer` pair
  (`announcer/gobgpv3/adapter_test.go`), covering IPv4/IPv6 × eBGP/iBGP.
* Write unit tests next to the code (`*_test.go`). Testability is achieved by
  injecting functions (e.g. `pingerFn`, `queryFn`, `interfaceCollector`).

## Configuration notes (quick reference)

* Only two environment variables: `CONFIG_PATH` (default `/config.yaml`) and
  `LOG_LEVEL` (default `WARN`). No CLI flags.
* Config file is YAML or JSON; parsed/validated by `config.NewFromFile`.
* Peer `remote_address` validation (`isSessionAddress`) rejects IPv6 link-local
  addresses, zone-scoped addresses, and IPv4-mapped IPv6 addresses.
* IPv4 routes require an IPv4 peer; IPv6 routes require an IPv6 peer.
* Multihop peers require the matching local address
  (`local_address` for IPv4, `local_address6` for IPv6).
* `LocalAddressFor(peer)` returns `""` for an unset/unspecified local address,
  letting GoBGP use the kernel route-lookup source.

## Security constraints (do not silently change)

* Peer authentication is **not** supported (no password/MD5/TLS in `config.Peer`).
* The metrics endpoint has **no TLS/auth/timeouts**
  (`http.ListenAndServe(addr, nil)`).
* GoBGP is started with `ListenPort: -1` (no external gRPC listener).
* The container runs as non-root `USER 65534:65534` in the Dockerfile.
* CI runs security linters (`gosec`, `govulncheck`) as non-failing steps. If you
  add auth, TLS, timeouts, or security tooling, document it in `SPEC.md`.

## Documentation rules

* `SPEC.md` and `AGENTS.md` are always in **English**.
* `README.md` example config, metric tables, and checker list must stay in
  sync with the code and with `SPEC.md`.
* Any metric added/removed/renamed must be reflected in `README.md` and
  `SPEC.md` (full `anycastd_` names and types).
