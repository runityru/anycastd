# anycastd

[![Verify](https://github.com/runityru/anycastd/actions/workflows/verify.yml/badge.svg?branch=master)](https://github.com/runityru/anycastd/actions/workflows/verify.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/runityru/anycastd)](https://goreportcard.com/report/github.com/runityru/anycastd)
[![Go Reference](https://pkg.go.dev/badge/github.com/runityru/anycastd.svg)](https://pkg.go.dev/github.com/runityru/anycastd)

anycastd is aimed to bring new age approaches to classic applications by allowing
to announce virtual addresses via BGP w/ service-level checks.

Let's imagine you're going to create your own DNS/HTTP/whatever else service
so how to utilize L3 load balancing and remove announce when something goes
wrong on particular node? anycastd is exactly for that! No bunch of dedicated
services and stateful control on BGP routes and no semi-tested Python scripts
anymore!

## Usage

anycastd is written as a pretty compact daemon like application so it could
run in two main ways: via systemd or via docker/podman/any other container
engine so that's why there's a container image in packages is available in
parallel with traditional Go binaries.

For monitoring purposes anycastd listens HTTP socket with Prometheus metrics
endpoint so once routes are removed from the announce SREs could go and check
why that happened.

## Configuration

anycastd aims to follow [12factor](https://12factor.net) principles however
it's not always possible to encode complex data structures into environment
variable. So configuration is divided into two ways:

### Environment

anycastd allows to set the following options via environment variables:

* `CONFIG_PATH` (string, default: `/config.yaml`) - path to the configuration file
* `LOG_LEVEL` (ENUM, default: `WARN`) - logging verbosity level, could one of the:
  * `TRACE`
  * `DEBUG`
  * `INFO`
  * `WARNING`
  * `ERROR`
  * `FATAL`
  * `PANIC`

### Configuration file

Configuration file contains service configuration, i.e. how to announce,
where to announce, checks to perform before announce. JSON and YAML formats
for the same data structure are supported. Example:

```yaml
---
announcer:
  engine: gobgpv3 # BGP engine: "gobgpv3" or "gobgpv4"
  router_id: 10.3.3.3
  local_address: 10.0.0.1
  local_address6: 2001:db8::1
  local_asn: 65999
  routes:
    - 10.0.0.128/32
    - 2001:db8:1::/48
  bfd: # BFD to each peer (GoBGP v4 only). Intervals are in milliseconds (ms).
    enabled: true
    port: 3784 # default 3784 for all peers; set 4784 explicitly for multihop
    min_tx_interval: 300ms # default GoBGP 1s; floor 50ms
    min_rx_interval: 300ms # default GoBGP 1s; floor 50ms
    multiplier: 3 # default 3; range 1..255
  peers:
    - name: some_router_1
      remote_address: 10.0.0.252
      remote_asn: 65000
    - name: some_router_2
      remote_address: 10.0.0.253
      remote_asn: 65000
      enable_multihop: true
      multihop_ttl: 3
    - name: some_router_3
      remote_address: 2001:db8::252
      remote_asn: 65000
services:
  - name: http
    check_interval: 10s
    checks:
      - kind: dns_lookup
        spec:
          query: google.com
          resolver: 127.0.0.1:53
          tries: 3
          interval: 100ms
          timeout: 3s
      - kind: http_2xx
        spec:
          url: http://127.0.0.1:8080/test-path
          method: GET
          headers:
            Host: example.com
          payload: ping
          tries: 3
          interval: 100ms
          timeout: 2s
      - kind: tls_certificate
        spec:
          local:
            path: /etc/ssl/pki/cert.pem
          common_name: Test certificate
          dns_names:
            - site.example.org
          ip_addresses:
            - 127.0.0.1
          issuer: Test Issuer
      - kind: assigned_address
        spec:
          interface: dummy0
          ipv4: 33.22.11.0
      - kind: icmp_ping
        spec:
          static:
            host: google.com
          tries: 3
          interval: 100ms
          timeout: 5s
      - kind: tftp_rrq
        spec:
          url: tftp://127.0.0.1:69/lpxelinux.0
          tries: 3
          interval: 100ms
          timeout: 5s
      - kind: ntpq
        spec:
          server: 0.ru.pool.ntp.org
          src_addr: 192.168.0.1
          tries: 3
          offset_threshold: 125ms
          interval: 100ms
          timeout: 5s
metrics:
  enabled: true
  address: 127.0.0.1:9090
```

### BFD

`announcer.bfd` enables Bidirectional Forwarding Detection (BFD) for fast
failure detection towards each BGP peer. BFD is **GoBGP v4 only**: enabling it
with `engine: gobgpv3` is a config error (fail-closed). A BFD session is
established to each peer's `remote_address`; for multihop peers (derived
automatically from `peer.enable_multihop`) BFD runs in multihop mode.

| Field              | Type       | Default            | Notes                                                                                                                                    |
| ------------------ | ---------- | ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`          | `bool`     | `false`            | Master switch for BFD                                                                                                                    |
| `port`             | `uint16`   | `3784`             | Destination UDP port sent to the peer; default `3784` for **all** peers; set `4784` explicitly for multihop (RFC 5883); range `1..65535` |
| `min_tx_interval`  | `duration` | GoBGP `1s`         | Minimum transmit interval. Config is in **milliseconds** (e.g. `300ms`); floor `50ms`                                                    |
| `min_rx_interval`  | `duration` | GoBGP `1s`         | Minimum receive interval. Config is in **milliseconds** (e.g. `300ms`); floor `50ms`                                                     |
| `multiplier`       | `uint32`   | `3`                | Detection multiplier; range `1..255` (max `255`)                                                                                         |

Multihop is **not** configured inside `bfd` — it is derived from the peer's
`enable_multihop` automatically.

**`port` semantics**: `announcer.bfd.port` is the **destination** port that
anycastd sends BFD packets **to the peer**. The default is **`3784` for all
peers, including multihop** — GoBGP v4 does not switch the destination port
based on `enable_multihop`. For a multihop peer talking to an
RFC 5883-compatible router that listens on `4784`, you must set
`port: 4784` explicitly; do not rely on a default. The GoBGP receiving socket
listens on **both `3784` and `4784`**, so inbound packets are accepted on
either. The setting is **global for all peers** — a different port cannot be
used for single-hop and multihop peers at the same time.

**Peer local address**: a BFD session can only be established when the peer has
a valid local address at `AddPeer` time. Operators enabling BFD should therefore
set the peer's `local_address` (IPv4) / `local_address6` (IPv6) explicitly.

**Units**: configuration values are durations expressed in milliseconds (e.g.
`300ms`); on the wire and in the GoBGP API BFD uses **microseconds**
(RFC 5880). Local detection time = `min_rx_interval × multiplier`
(300ms × 3 ≈ 900ms failover).

**Behavior on BFD-down**: GoBGP performs a hard reset of the peer whose BFD
session went down. The prefix is simply not delivered to that peer; all other
peers keep receiving it. anycastd does **not** withdraw the announcement.

**Environment requirements**: BFD needs UDP `3784` (single-hop) / `4784`
(multihop) in both directions plus ephemeral ports `49152–65535`. **No root is
required** (ports are > 1024 and the container runs as `USER 65534`). BFD must
be enabled on **both** sides to come up. Like BGP, BFD is **not
authenticated**.

## GoBGP engine & security notice

anycastd can run on **GoBGP v3** (`engine: gobgpv3`) or **GoBGP v4**
(`engine: gobgpv4`). Operators should be aware of the following security
considerations:

- **GoBGP v4** (pinned `v4.11.0`) is **not** affected by
  [CVE-2026-30405](https://github.com/osrg/gobgp/security/advisories/GHSA-4p9m-8gc4-rw2h)
  / [GO-2026-4736](https://vuln.go.dev/). The fix landed in v4.4.0
  (commits `f12b8fb`, `583080a`) and is present in the pinned version.
- **GoBGP v3** (pinned `v3.37.0`) **is affected** by CVE-2026-30405 and has
  **no fixed version** — the fix was **not** backported to the v3 line. The
  vulnerability is a remote, unauthenticated denial of service: a
  malformed BGP `UPDATE` carrying a `NEXT_HOP` attribute shorter than 4 bytes
  (with a session configured to treat errors as withdrawals) can panic the
  `fsmHandler.recvMessageloop` and crash the daemon. anycastd is a BGP
  speaker and accepts `UPDATE`s from its configured peers, so a malicious or
  compromised peer could take the process down.

**Recommendations:**

- Prefer `engine: gobgpv4`. It is not only free of this CVE but also the only
  engine that supports BFD (see above). Migrate any `gobgpv3` deployments to
  `gobgpv4`.
- If `gobgpv3` must be used, narrow the attack surface: restrict BGP peering
  (TCP 179) to trusted peer addresses only (host ACL/firewall, network
  policies) and run the daemon under a supervisor that restarts it on crash.
- Note: `govulncheck` may report `Fixed in: N/A` for **both** engines. This is
  an artifact of the Go vulnerability database entry `GO-2026-4736` (affected
  range declared with no `fixed` event) and does **not** mean the v4 fix is
  missing — the v4.11.0 source contains both patches.

## Available checks

Check (implemented via Checker interface) is core concept in anycastd, allows
writing well-tested piece of code to enable or disable announce. In anycastd
most of checks could be written without any shell invocations i.e. in pure Go
which is a preferable way. That's why there's no exec command check ;)

For now the following checks are available:

* assigned_address - ensures the address is assigned on interface
* dns_lookup - performs DNS lookup
* http_2xx - performs HTTP check and expects 2xx code
* icmp_ping - performs ICMP ping to the specified host
* tftp_rrq - performs TFTP GET request to specified URL
* ntpq - checks local clock offset against remote NTP server from specific addr
* tls_certificate - performs TLS certificate validation & provide expiration
  date via metrics

## Metrics

A special part of anycastd is a Prometheus-compatible metrics allows monitoring
systems to notify operators about any changes in their service.

For now anycastd provides the following metrics:

| Metric name | Labels  | Type  | Description                                    |
| ----------- | ------- | ----- | ---------------------------------------------- |
| anycastd_up | version | gauge | Application liveness status (must always be 1) |

### Service

Service could provide their metrics in order to aggregate current statuses.

| Metric name         | Labels  | Type  | Description                             |
| ------------------- | ------- | ----- | --------------------------------------- |
| anycastd_service_up | service | gauge | Service liveness status based on checks |

### Checks

Check running engine provides the following metrics.

| Metric name                     | Labels         | Type  | Description                            |
| ------------------------------- | -------------- | ----- | -------------------------------------- |
| anycastd_check_duration_seconds | service, check | gauge | Duration of check execution in seconds |

In addition some checkers could provide their own metrics the list of them is below:

#### tls_certificate check

| Metric name                             | Labels      | Type  | Description                                  |
| --------------------------------------- | ----------- | ----- | -------------------------------------------- |
| anycastd_certificate_expires_in_seconds | check, path | gauge | Time the certificate expires in (in seconds) |

#### icmp_ping check

| Metric name                                      | Labels      | Type    | Description                                |
| ------------------------------------------------ | ----------- | ------- | ------------------------------------------ |
| anycastd_check_avg_rtt_seconds                   | check, host | gauge   | Avg RTT of ICMP checks                     |
| anycastd_check_loss_percent                      | check, host | gauge   | Percent of packet loss                     |
| anycastd_check_max_rtt_seconds                   | check, host | gauge   | Max RTT of ICMP checks                     |
| anycastd_check_min_rtt_seconds                   | check, host | gauge   | Min RTT of ICMP checks                     |
| anycastd_check_packets_received_duplicates_total | check, host | counter | Total amount of duplicate packets received |
| anycastd_check_packets_received_total            | check, host | counter | Total amount of packets received           |
| anycastd_check_packets_sent_total                | check, host | counter | Total amount of packets sent               |
| anycastd_check_std_dev_rtt_seconds               | check, host | gauge   | Standard deviation RTT of ICMP checks      |

#### ntpq check

| Metric name                            | Labels                | Type    | Description                                                                   |
| -------------------------------------- | --------------------- | ------- | ----------------------------------------------------------------------------- |
| anycastd_check_last_ntp_offset_seconds | check, host, src_addr | gauge   | The estimated offset of the local system clock relative to the server's clock |
| anycastd_check_last_ntp_rtt_seconds    | check, host, src_addr | gauge   | An estimate of the round-trip-time delay between the client and the server    |
| anycastd_check_ntp_packets_sent_total  | check, host, src_addr | counter | Total amount of ntp packets sent                                              |
| anycastd_check_ntp_unverified_total    | check, host, src_addr | counter | Total amount of checks that could not verify the local clock                  |

### GoBGP

The core of anycastd for BGP communication is GoBGP which allows so gather
some details about peers, sessions and announces.

| Metric name                                      | Labels          | Type  | Description                                                                                         |
| ------------------------------------------------ | --------------- | ----- | --------------------------------------------------------------------------------------------------- |
| anycastd_gobgp_peer_admin_state                  | router_id, peer | gauge | Peer state 0=up, 1=down, 2=pfx_ct                                                                   |
| anycastd_gobgp_peer_bfd_state                    | router_id, peer | gauge | BFD session state of the peer 0=down, 1=up (via GoBGP ListBfdPeer)                                  |
| anycastd_gobgp_peer_count                        | router_id       | gauge | Total amount of peers configured for the GoBGP instance                                             |
| anycastd_gobgp_peer_flops_count                  | router_id, peer | gauge | Peer flops count                                                                                    |
| anycastd_gobgp_peer_out_queue_count              | router_id, peer | gauge | Peer outgoing messages queue                                                                        |
| anycastd_gobgp_peer_password_set_flag            | router_id, peer | gauge | Whether the peer have peer password set flag set                                                    |
| anycastd_gobgp_peer_remove_private_flag          | router_id, peer | gauge | Whether the peer have remove private flag set                                                       |
| anycastd_gobgp_peer_send_community_flag          | router_id, peer | gauge | Whether the peer have send community flag set                                                       |
| anycastd_gobgp_peer_session_state                | router_id, peer | gauge | Peer session state 0=unknown, 1=idle, 2=connect, 3=active, 4=opensent, 5=openconfirm, 6=established |
| anycastd_gobgp_peer_type                         | router_id, peer | gauge | Peer type 0=internal, 1=external                                                                    |
| anycastd_gobgp_received_messages_keepalive       | router_id, peer | gauge | Number of Keepalive messages received from the peer                                                 |
| anycastd_gobgp_received_messages_notification    | router_id, peer | gauge | Number of Notification messages received from the peer                                              |
| anycastd_gobgp_received_messages_open            | router_id, peer | gauge | Number of Open messages received from the peer                                                      |
| anycastd_gobgp_received_messages_refresh         | router_id, peer | gauge | Number of Refresh messages received from the peer                                                   |
| anycastd_gobgp_received_messages_total           | router_id, peer | gauge | Total number of messages received from the peer                                                     |
| anycastd_gobgp_received_messages_update          | router_id, peer | gauge | Number of Update messages received from the peer                                                    |
| anycastd_gobgp_received_messages_withdraw_update | router_id, peer | gauge | Number of Withdraw Update messages received from the peer                                           |
| anycastd_gobgp_received_messages_withdraw_prefix | router_id, peer | gauge | Number of Withdraw Prefix messages received from the peer                                           |
| anycastd_gobgp_sent_messages_keepalive           | router_id, peer | gauge | Number of Keepalive messages sent to the peer                                                       |
| anycastd_gobgp_sent_messages_notification        | router_id, peer | gauge | Number of Notification messages sent to the peer                                                    |
| anycastd_gobgp_sent_messages_open                | router_id, peer | gauge | Number of Open messages sent to the peer                                                            |
| anycastd_gobgp_sent_messages_refresh             | router_id, peer | gauge | Number of Refresh messages sent to the peer                                                         |
| anycastd_gobgp_sent_messages_total               | router_id, peer | gauge | Total number of messages sent to the peer                                                           |
| anycastd_gobgp_sent_messages_update              | router_id, peer | gauge | Number of Update messages sent to the peer                                                          |
| anycastd_gobgp_sent_messages_withdraw_prefix     | router_id, peer | gauge | Number of Withdraw Prefix messages sent to the peer                                                 |
| anycastd_gobgp_sent_messages_withdraw_update     | router_id, peer | gauge | Number of Withdraw Update messages sent to the peer                                                 |
