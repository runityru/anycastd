# anycastd — Architecture Specification

## 1. Introduction

`anycastd` is a compact daemon that announces and withdraws virtual (anycast)
addresses over BGP based on the health of the services running behind them.
It implements L3 load balancing without a dedicated, stateful control plane:
when a service on a node is healthy, its prefixes are announced to configured
BGP peers; when the service becomes unhealthy, the prefixes are withdrawn so
traffic is steered to another node.

The daemon is written in Go and uses [GoBGP v3](https://github.com/osrg/gobgp)
as its embedded BGP engine. The module path is `github.com/runityru/anycastd`.

### Goals

- Announce anycast prefixes only while a service is healthy.
- Withdraw announced prefixes promptly when checks start failing.
- Rely on service-level health checks written in pure Go (no shelling out).
- Expose Prometheus metrics so operators can observe announcements and peers.
- Follow [12-factor](https://12factor.net) principles as far as a structured
  YAML/JSON configuration file allows.

## 2. High-Level Architecture

The daemon is composed of several packages. Startup and wiring happen in
`cmd/anycastd/main.go`; the functional building blocks live in dedicated
packages.

```
+--------------------------------------------------------------+
| cmd/anycastd/main.go                                          |
|   - envconfig (CONFIG_PATH, LOG_LEVEL)                        |
|   - loads config, wires errgroup, signals                     |
|   - starts GoBGP server, adds peers, starts services          |
+--------------------------------------------------------------+
        |                     |                    |
        v                     v                    v
+-------------+       +-------------+       +--------------+
|  announcer  |       |  checkers   |       |   service    |
|  (BGP)      |       |  (health)   |       |  (orchestr.) |
|  + gobgpv3  |       |  registry   |       |  metrics     |
+-------------+       +-------------+       +--------------+
```

### Components

- **`cmd/anycastd`** — entry point. Reads environment variables, loads and
  validates the configuration file, initializes the GoBGP server, registers
  peers, builds one `service.Service` per configured service, and (optionally)
  starts the metrics HTTP server and the GoBGP metrics poller.
- **`announcer`** — defines the `GoBGPServer` and `Announcer` interfaces and an
  implementation that iterates over configured prefixes, calling `AddPath` /
  `DeletePath` for each. Also hosts the GoBGP peer/session metrics poller.
- **`announcer/gobgpv3`** — the GoBGP v3 adapter. Implements `GoBGPServer` by
  wrapping a `*server.BgpServer` and isolating all GoBGP-specific types.
- **`checkers`** — a registry of health check implementations, keyed by
  `kind`, with `Register` / `MustRegister` / `NewCheckerByKind`. Each checker
  implements the `Checker` interface.
- **`service`** — the orchestration layer. Runs the check loop for a service,
  applies a strategy over the check results, decides whether the service is up
  or down, and drives the announcer accordingly. Also owns the core metrics.
- **`config`** — YAML/JSON configuration model with validation.

## 3. Configuration

### 3.1 Environment variables

The process is configured exclusively via environment variables through
`kelseyhightower/envconfig`. There are **no command-line flags**.

| Variable      | Type        | Default        | Description                       |
| ------------- | ----------- | -------------- | --------------------------------- |
| `CONFIG_PATH` | `string`    | `/config.yaml` | Path to the YAML/JSON config file |
| `LOG_LEVEL`   | `log.Level` | `WARN`         | Logging verbosity (logrus levels) |

Log output uses a logrus `TextFormatter` with `FullTimestamp = true`.

### 3.2 Configuration file

The file is selected by `CONFIG_PATH` and may be **YAML** (`.yml` / `.yaml`) or
**JSON** (`.json`). Any other extension is rejected. YAML is converted to JSON
via an intermediate representation and then parsed with the same JSON decoding
path. The file is parsed and validated by `config.NewFromFile`, which returns
an error on any validation failure.

Top-level structure:

```yaml
announcer:
  router_id: 10.3.3.3 # required, IPv4
  local_address: 10.0.0.1 # optional, IPv4
  local_address6: 2001:db8::1 # optional, IPv6
  local_asn: 65999 # required, uint32
  routes: # required, list of CIDR prefixes
    - 10.0.0.128/32
    - 2001:db8:1::/48
  peers: # required, at least one
    - name: some_router_1
      remote_address: 10.0.0.252
      remote_asn: 65000
services: # required, at least one
  - name: http
    check_interval: 10s # required
    strategy: at_least_one # optional, default "at_least_one"
    strategy_options: {} # optional, JSON blob
    checks: # required, at least one
      - kind: http_2xx # required
        spec: { ... } # required, JSON blob
        group: "" # optional
metrics:
  enabled: true # required
  address: 127.0.0.1:9090 # required
```

### 3.3 `announcer`

| Field            | Type       | Required | Validation                                                                                  |
| ---------------- | ---------- | -------- | ------------------------------------------------------------------------------------------- |
| `router_id`      | `string`   | yes      | IPv4 address                                                                                |
| `local_address`  | `string`   | no       | IPv4, must be a valid _session_ address (see below)                                         |
| `local_address6` | `string`   | no       | IPv6, must be a valid _session_ address (see below)                                         |
| `local_asn`      | `uint32`   | yes      | Local AS number                                                                             |
| `routes`         | `[]string` | yes      | Each entry a valid CIDR; IPv4 route requires an IPv4 peer, IPv6 route requires an IPv6 peer |
| `peers`          | `[]Peer`   | yes      | At least one peer; multihop peers need a local address                                      |

**Session-address validation** (`isSessionAddress`): the address must be a
valid IP, must **not** be an IPv6 link-local address, must **not** carry a
zone, and must **not** be an IPv4-mapped IPv6 address. GoBGP mishandles these
cases: IPv6 link-local addresses need a zone, a zone yields a `::` next hop,
and an IPv4-mapped IPv6 address is treated as IPv4.

**`LocalAddressFor(peer)`** returns the local address of the peer's address
family (`local_address` for IPv4 peers, `local_address6` for IPv6 peers), or
`""` when the value is unset or **unspecified**. The empty string tells GoBGP
to use the kernel route-lookup source address for the session.

### 3.4 `peer`

| Field             | Type     | Required | Validation                                                                        |
| ----------------- | -------- | -------- | --------------------------------------------------------------------------------- |
| `name`            | `string` | yes      | Peer name                                                                         |
| `remote_address`  | `string` | yes      | Valid session address (not IPv6 link-local, no zone, not 4-in-6, not unspecified) |
| `remote_asn`      | `uint32` | yes      | Remote AS number                                                                  |
| `enable_multihop` | `bool`   | no       | Enable eBGP multihop                                                              |
| `multihop_ttl`    | `uint32` | no       | `>= 1` when `enable_multihop` is true; otherwise unvalidated                      |

An IPv6 peer (`remote_address` parses as IPv6) selects the IPv6 local address.
When `enable_multihop` is set and no local address is available, an IPv6 peer
requires `local_address6`; an IPv4 peer requires `local_address`.

### 3.5 `service`

| Field              | Type            | Required | Notes                                                                                         |
| ------------------ | --------------- | -------- | --------------------------------------------------------------------------------------------- |
| `name`             | `string`        | yes      | Service name (used in metrics labels)                                                         |
| `check_interval`   | `time.Duration` | yes      | Interval between check rounds                                                                 |
| `strategy`         | `string`        | no       | One of `all`, `at_least_one`, `all_in_group`, `at_least_n_percentage`; default `at_least_one` |
| `strategy_options` | JSON blob       | no       | Consumed by strategies that need parameters (e.g. `n` for `at_least_n_percentage`)            |
| `checks`           | `[]Check`       | yes      | At least one check                                                                            |

**Strategies** (implemented in `service/strategy.go`, type `Strategy func([]CheckResult) (bool, error)` where the returned `bool` means _service is down_):

- `all` — service is down only if **every** check failed.
- `at_least_one` (default) — service is down if **any** check failed.
- `all_in_group` — service is down if **every** check in any group failed;
  checks are grouped by their `group` field. Checks with an empty `group`
  value are all aggregated into a single (empty) group.
- `at_least_n_percentage` — service is down if the ratio of failed checks
  exceeds `n` (parsed from `strategy_options` JSON `{"n": <float>}`).

### 3.6 `check`

| Field   | Type      | Required | Notes                                          |
| ------- | --------- | -------- | ---------------------------------------------- |
| `kind`  | `string`  | yes      | Registered checker kind                        |
| `spec`  | JSON blob | yes      | Checker-specific configuration                 |
| `group` | `string`  | no       | Group name used by the `all_in_group` strategy |

### 3.7 `metrics`

| Field     | Type     | Required | Notes                                                             |
| --------- | -------- | -------- | ----------------------------------------------------------------- |
| `enabled` | `bool`   | yes      | Whether to start the metrics HTTP server and GoBGP metrics poller |
| `address` | `string` | yes      | `host:port` for the Prometheus `/metrics` endpoint                |

## 4. Data Flows

### 4.1 Startup (cmd/anycastd/main.go)

1. Register a signal context for `SIGINT` / `SIGTERM`.
2. Read `CONFIG_PATH` / `LOG_LEVEL` via `envconfig.MustProcess`.
3. Load and validate the configuration (`config.NewFromFile`).
4. Configure logrus (level + `TextFormatter{FullTimestamp: true}`).
5. Create an `errgroup` with the signal context.
6. Create the GoBGP `server.BgpServer` (wrapped with a custom `announcer.Logger`)
   and run `bgpSrv.Serve()` in the errgroup.
7. `StartBgp` with the configured `RouterId`, `Asn`, and `ListenPort: -1`
   (the embedded gRPC server does not listen on an external port).
8. Watch peer events (`WatchEventRequest.Peer`) for logging.
9. Add each configured peer via `AddPeer`.
10. For each service:
    - build checkers from the registry,
    - create an `announcer.New` with a `gobgpv3.NewAdapter(bgpSrv)` and the
      configured routes,
    - resolve the strategy,
    - create and run a `service.New(...)` in the errgroup.
11. If metrics are enabled:
    - register the Prometheus handler and `http.ListenAndServe(metrics.address, nil)`,
    - create and run `announcer.NewMetricsRepository(...)` (GoBGP metrics poller).
12. `g.Wait()` — block until the context is cancelled or a goroutine errors.

### 4.2 Announce / denounce lifecycle (service)

The `service.Service.Run` loop ticks every `check_interval` and runs one
`run` pass:

1. For each configured check, run it through `metrics.MeasureCall`, which times
   the call and records `anycastd_check_duration_seconds{service,check}`. A
   check that returns an error is recorded as **failed**.
2. Feed the collected `CheckResult` list to the strategy. The strategy returns
   `serviceDown` (bool).
3. Record `anycastd_service_up{service}` as `1`/`0`.
4. If `serviceDown` and the announcer was previously in the announced state
   (`announced *atomic.Bool`), call `announcer.Denounce(ctx)` and clear the flag.
5. If the service is up and not currently announced, call
   `announcer.Announce(ctx)` and set the flag.

This is **stateful**: prefixes are only re-announced/withdrawn on a state
transition, not on every tick.

### 4.3 Announce / denounce mechanics (announcer)

`announcer.Announce(ctx)` iterates over all configured prefixes and calls
`GoBGPServer.AddPath(ctx, prefix)` for each. `Denounce` does the same with
`DeletePath`. The underlying adapter (`gobgpv3`) builds a GoBGP v3 `api.Path`
for each prefix and calls the GoBGP server's `AddPath` / `DeletePath`.

## 5. BGP Behavior (gobgpv3 adapter)

`gobgpv3.NewAdapter(*server.BgpServer)` returns an `announcer.GoBGPServer`.

For each prefix, `newPath`:

- Parses the prefix with `netip.ParsePrefix`.
- Selects the address family by the address type:
  - IPv4 → `AFI_IP`, next hop `0.0.0.0` (`netip.IPv4Unspecified()`),
  - IPv6 → `AFI_IP6`, next hop `::` (`netip.IPv6Unspecified()`).
- Builds the NLRI as an `IPAddressPrefix` with the prefix address and length.
- Attaches path attributes:
  - `OriginAttribute{Origin: 0}` (IGP origin),
  - `NextHopAttribute{NextHop: <unspecified>}`.
- Returns a `api.Path` with `Family: {Afi, Safi: SAFI_UNICAST}`.

The **unspecified next hop** is intentional: GoBGP replaces it with each
session's local address when advertising to a peer, so one path object serves
all peers of a family. Dual-stack is supported: a prefix is announced in the
address family of its own address, and a session's local address (from
`LocalAddressFor`) is used as the source.

**Peer configuration** (`newPeer` in main.go) builds a GoBGP `api.Peer` with:

- `PeerConf{NeighborAddress, PeerAsn}` — no authentication fields are set.
- `EbgpMultihop{Enabled, MultihopTtl}`.
- `Transport{LocalAddress}` from `Announcer.LocalAddressFor(peer)`.
- `Timers.Config{ConnectRetry: 5}` — GoBGP's default 120 s retry is too slow
  for a tentative IPv6 bind waiting for duplicate-address detection (DAD).

## 6. Checkers

Checkers implement the `Checker` interface:

```go
type Checker interface {
    Kind() string
    Check(ctx context.Context) error
}
```

They are registered into the package-level `registry` (a `map[string]func(json.RawMessage) (Checker, error)`
guarded by an `RWMutex`) via `checkers.Register` / `checkers.MustRegister` in
each package's `init()`, and constructed at runtime by `checkers.NewCheckerByKind`.
New checkers are wired into the binary by blank-importing their package in
`cmd/anycastd/main.go`.

The seven built-in checkers are:

| Kind               | Purpose                                                                                                                   | Metrics (see §7) |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| `assigned_address` | Ensures the IPv4 address is assigned, optionally on a named interface (`interface`, `ipv4`)                               | —                |
| `dns_lookup`       | Performs a DNS lookup for `query` against `resolver` (`tries`, `interval`, `timeout`)                                     | —                |
| `http_2xx`         | Performs an HTTP request and expects a 2xx status (`url`, `method`, `headers`, `payload`, `tries`, `interval`, `timeout`) | —                |
| `icmp_ping`        | Pings `static.host` (`tries`, `interval`, `timeout`); fails on packet loss                                                | Yes              |
| `ntpq`             | Checks the local clock offset against a remote NTP server from `src_addr` (`offset_threshold`)                            | Yes              |
| `tftp_rrq`         | Performs a TFTP read request (RRQ) to `url`; optionally validates `expected_sha256`                                       | —                |
| `tls_certificate`  | Validates a TLS certificate (local file or remote host) and exposes expiry via metrics                                    | Yes              |

### `tls_certificate` spec

- `local: {path}` or `remote: {host, insecure}` — exactly one is required.
- `common_name`, `dns_names`, `ip_addresses`, `issuer` — optional expectations
  checked against the server certificate.

### `icmp_ping` spec

- `static: {host}` — the target host.
- `tries`, `interval`, `timeout`.

### `ntpq` spec

- `server` (host or IP), `src_addr` (source IP; family must match an IP-literal server).
- `tries`, `offset_threshold`, `interval`, `timeout`.
- A verified offset exceeding the threshold withdraws the route; an unverified
  or missing reply leaves the route announced (only a verified failure
  withdraws).

## 7. Prometheus Metrics

All metrics use the `anycastd` namespace. Core gauges are registered in
`service/metrics.go`; GoBGP peer metrics in `announcer/metrics.go`; checker
metrics in their respective packages. There is no authentication or TLS on the
metrics endpoint.

### 7.1 Core metrics (service)

| Metric                            | Type  | Labels             | Description                                            |
| --------------------------------- | ----- | ------------------ | ------------------------------------------------------ |
| `anycastd_up`                     | gauge | `version`          | Application liveness status (always `1` while running) |
| `anycastd_service_up`             | gauge | `service`          | Service liveness status based on checks (`1`/`0`)      |
| `anycastd_check_duration_seconds` | gauge | `service`, `check` | Duration of the last check execution in seconds        |

### 7.2 GoBGP metrics (announcer)

Registered with namespace `anycastd`, subsystem `gobgp`. All are gauges.
Labels: `peer_count` uses `router_id`; all others use `router_id, peer`. They
are polled every **1 second** by `announcer.NewMetricsRepository(...).Run`.

| Metric name                                        | Labels            |
| -------------------------------------------------- | ----------------- |
| `anycastd_gobgp_peer_count`                        | `router_id`       |
| `anycastd_gobgp_peer_admin_state`                  | `router_id, peer` |
| `anycastd_gobgp_peer_session_state`                | `router_id, peer` |
| `anycastd_gobgp_peer_out_queue_count`              | `router_id, peer` |
| `anycastd_gobgp_peer_flops_count`                  | `router_id, peer` |
| `anycastd_gobgp_peer_send_community_flag`          | `router_id, peer` |
| `anycastd_gobgp_peer_remove_private_flag`          | `router_id, peer` |
| `anycastd_gobgp_peer_password_set_flag`            | `router_id, peer` |
| `anycastd_gobgp_peer_type`                         | `router_id, peer` |
| `anycastd_gobgp_received_messages_total`           | `router_id, peer` |
| `anycastd_gobgp_received_messages_notification`    | `router_id, peer` |
| `anycastd_gobgp_received_messages_update`          | `router_id, peer` |
| `anycastd_gobgp_received_messages_open`            | `router_id, peer` |
| `anycastd_gobgp_received_messages_keepalive`       | `router_id, peer` |
| `anycastd_gobgp_received_messages_refresh`         | `router_id, peer` |
| `anycastd_gobgp_received_messages_withdraw_update` | `router_id, peer` |
| `anycastd_gobgp_received_messages_withdraw_prefix` | `router_id, peer` |
| `anycastd_gobgp_sent_messages_total`               | `router_id, peer` |
| `anycastd_gobgp_sent_messages_notification`        | `router_id, peer` |
| `anycastd_gobgp_sent_messages_update`              | `router_id, peer` |
| `anycastd_gobgp_sent_messages_open`                | `router_id, peer` |
| `anycastd_gobgp_sent_messages_keepalive`           | `router_id, peer` |
| `anycastd_gobgp_sent_messages_refresh`             | `router_id, peer` |
| `anycastd_gobgp_sent_messages_withdraw_update`     | `router_id, peer` |
| `anycastd_gobgp_sent_messages_withdraw_prefix`     | `router_id, peer` |

`peer_password_set_flag` is set from `peerState.GetAuthPassword()`; because
anycastd does not configure peer authentication (see §8), it reads the flag
straight from the GoBGP peer state and is normally `0`.

### 7.3 Checker metrics

**`tls_certificate`:**

| Metric                                    | Type  | Labels          | Description                           |
| ----------------------------------------- | ----- | --------------- | ------------------------------------- |
| `anycastd_certificate_expires_in_seconds` | gauge | `check`, `path` | Seconds until the certificate expires |

**`icmp_ping`:**

| Metric                                             | Type    | Labels          | Description                           |
| -------------------------------------------------- | ------- | --------------- | ------------------------------------- |
| `anycastd_check_max_rtt_seconds`                   | gauge   | `check`, `host` | Max RTT of ICMP checks                |
| `anycastd_check_min_rtt_seconds`                   | gauge   | `check`, `host` | Min RTT of ICMP checks                |
| `anycastd_check_avg_rtt_seconds`                   | gauge   | `check`, `host` | Average RTT of ICMP checks            |
| `anycastd_check_std_dev_rtt_seconds`               | gauge   | `check`, `host` | Standard deviation RTT of ICMP checks |
| `anycastd_check_loss_percent`                      | gauge   | `check`, `host` | Percent of packet loss                |
| `anycastd_check_packets_sent_total`                | counter | `check`, `host` | Total packets sent                    |
| `anycastd_check_packets_received_total`            | counter | `check`, `host` | Total packets received                |
| `anycastd_check_packets_received_duplicates_total` | counter | `check`, `host` | Total duplicate packets received      |

**`ntpq`:**

| Metric                                   | Type    | Labels                      | Description                                         |
| ---------------------------------------- | ------- | --------------------------- | --------------------------------------------------- |
| `anycastd_check_last_ntp_offset_seconds` | gauge   | `check`, `host`, `src_addr` | Estimated local clock offset relative to server     |
| `anycastd_check_last_ntp_rtt_seconds`    | gauge   | `check`, `host`, `src_addr` | Estimated round-trip time between client and server |
| `anycastd_check_ntp_packets_sent_total`  | counter | `check`, `host`, `src_addr` | Total NTP packets sent                              |
| `anycastd_check_ntp_unverified_total`    | counter | `check`, `host`, `src_addr` | Total checks that could not verify the clock        |

## 8. Security Model

The security posture is **honest and limited by design**:

- **No peer authentication.** `config.Peer` has no password/MD5/TLS field, and
  `newPeer` does not populate `PeerConf.Auth`. The
  `anycastd_gobgp_peer_password_set_flag` metric merely _reads_ the flag from
  GoBGP and does not enable anything. BGP sessions are unauthenticated; secure
  the control plane at the network layer (e.g. ACLs / isolated network).
- **TLS exists only as a health check** (`tls_certificate`), not as transport
  security for the daemon's own endpoints. Note that `tls_certificate` accepts
  `remote.insecure: true`, which **disables server-certificate verification**
  for that check — treat it as security-relevant and use it only for
  testing/internal targets.
- **Open metrics endpoint.** The metrics HTTP server is started with
  `http.ListenAndServe(metrics.address, nil)` — **no TLS, no authentication,
  no read/write timeouts**. Bind it to a loopback or protected interface.
- **Configuration contains no secrets.** Only `CONFIG_PATH` and `LOG_LEVEL`
  are read from the environment; the config file holds no credentials.
- **No external gRPC listener.** GoBGP is started with `ListenPort: -1`, so the
  embedded gRPC server does not listen on an external port; anycastd drives it
  in-process.
- **Container runs as root.** The `Dockerfile` does not define a `USER`
  directive, so the process runs as root inside the scratch container.
- **CI does not run security linters.** The `verify` / `release` workflows run
  hadolint, markdownlint, golangci-lint, unit tests, and a goreleaser snapshot
  / Docker build, but no dedicated security scanning (e.g. gosec, Trivy).

## 9. Build, Testing and CI/CD

### 9.1 Module and Go version

- Module: `github.com/runityru/anycastd`
- `go 1.27.2`
- Key dependencies:
  - `github.com/osrg/gobgp/v3 v3.37.0`
  - `github.com/prometheus/client_golang v1.25.0`
  - `github.com/sirupsen/logrus v1.10.2`
  - `github.com/kelseyhightower/envconfig v1.4.0`
  - `github.com/go-ozzo/ozzo-validation/v4 v4.5.0`
  - `github.com/stretchr/testify v1.12.1`
  - `github.com/teran/go-time v0.0.2`, `github.com/teran/go-ptr v1.1.0`
  - `gopkg.in/yaml.v3 v3.0.1`, `golang.org/x/sync`, `google.golang.org/protobuf`

### 9.2 Build / test commands

```sh
go build ./...          # build all packages
go test ./...           # run all unit tests
golangci-lint run       # static analysis (via golangci-lint)
```

There is no Makefile; commands are invoked directly (also in CI).

### 9.3 Release build (.goreleaser.yaml)

- goreleaser config `version: 2`.
- Single build `anycastd` from `./cmd/anycastd`, binary name `anycastd`.
- `CGO_ENABLED=0`.
- Target `linux` for `amd64` (with `goamd64: v1, v2, v3`) and `arm64`
  (with `goarm: 7`).
- `ldflags`: `-s -w -X main.appVersion={{.Version}} -X main.buildTimestamp={{.Date}}`.
- Archive format `binary`; SHA-256 `checksums.txt`.

### 9.4 Container image (Dockerfile)

- Build stage from `alpine:3.24.1` to fetch `ca-certificates`.
- Final image is `scratch`; only `/etc/ssl/cert.pem` and the static binary
  `/anycastd` are copied in.
- `ENTRYPOINT ["/anycastd"]`; no `USER` directive (runs as root).

### 9.5 CI/CD workflows

- **`verify.yml`** — on push to `master` and on pull requests: hadolint,
  markdownlint, golangci-lint (Go `1.27.x`, protoc `23.x`), unit tests
  (`go test ./...`), a goreleaser `build --snapshot`, and a Docker build
  (`push: false`, platform `amd64`).
- **`release.yml`** — on tags `v*`: the same checks (hadolint, markdownlint,
  golangci-lint, unit tests + `go build -v ./...`), then goreleaser
  `release --clean`, and a Docker build **pushed** to GHCR (`ghcr.io/<repo>`
  tagged `latest`, the git tag, and the tag + timestamp).

## 10. Testing Approach

- Unit tests live next to the code (`*_test.go`).
- Interfaces are mocked with **hand-written `testify/mock` mocks** (e.g.
  `checkers/mock.go`, `announcer/mock.go`, `announcer/gobgp_mock.go`) — there is
  **no `mockgen`**.
- The GoBGP adapter is tested against a real in-memory `server.BgpServer` pair
  (`adapter_test.go`), asserting the next hop received by a peer BGP server for
  both IPv4 and IPv6, eBGP and iBGP.
