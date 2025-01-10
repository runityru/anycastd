# anycastd

[![Verify](https://github.com/runityru/anycastd/actions/workflows/verify.yml/badge.svg?branch=master)](https://github.com/runityru/anycastd/actions/workflows/verify.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/runityru/anycastd)](https://goreportcard.com/report/github.com/runityru/anycastd)
[![Go Reference](https://pkg.go.dev/badge/github.com/runityru/anycastd.svg)](https://pkg.go.dev/github.com/runityru/anycastd)

anycastd is aimed to bring new age approaches to classic applications by allowing
to announce virtual addresses via BGP w/ service-level checks.

Let's imaging you gonna create your own DNS/HTTP/whatever else service so how
to utilize L3 load balancing and remove announce when something goes wrong on
particular node? anycastd is exactly for that! No bunch of dedicated services
and stateful control on BGP routes and no semi-tested Python scripts anymore!

## Usage

anycastd is written as a pretty compact daemon like application so it could
run in two main ways: via systemd or via docker/podman/any other container
engine so that's why there's a container image in packages is available in
parallel with traditional Go binaries.

For monitoring purposes anycastd listens HTTP socket with Prometheus metrics
endpoint so once routes are removed from the announce SRE's could go and check
why that happened.

## Configuration

anycastd longs to follow [12factor](https://12factor.net) principles however
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
for the same data structure is supported. Example:

```yaml
---
announcer:
  router_id: 10.3.3.3
  local_address: 10.0.0.1
  local_asn: 65999
  routes:
    - 10.0.0.128/32
  peers:
    - name: some_router_1
      remote_address: 10.0.0.252
      remote_asn: 65000
    - name: some_router_2
      remote_address: 10.0.0.253
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

## Available checks

Check (implemented via Checker interface) is core concept in anycastd, allows
to write well-tested piece of code to enable or disable announce. In anycastd
most of checks could be written without any shell invocations i.e. in pure Go
which is a preferable way. That's why there's no exec command check ;)

For now the following checks are available:

* assigned_address - ensures the address is assigned on interface
* dns_lookup - performs DNS lookup
* http_2xx - performs HTTP check and expects 2xx code
* icmp_ping - performs ICMP ping to the specified host
* tftp_rrq - performs TFTP GET request to specified URL
* ntpq - performs NTP query to specific ntp server from specific src addr, checking offset 
* tls_certificate - performs TLS certificate validation & provide expiration
    date via metrics

## Metrics

A special part of anycastd is a Prometheus-compatible metrics allows monitoring
systems to notify operators about any changes in their service.

For now anycastd provides the following metrics:

| Metric name | Labels  | Description                                    |
|-------------|---------|------------------------------------------------|
| anycastd_up | version | Application liveness status (must always be 1) |

### Service

Service could provide their metrics in order to aggregate current statuses.

| Metric name         | Labels         | Description                             |
|---------------------|----------------|-----------------------------------------|
| anycastd_service_up | service, check | Service liveness status based on checks |

### Checks

Check running engine provides the following metrics.

| Metric name                     | Labels         | Description                            |
|---------------------------------|----------------|----------------------------------------|
| anycastd_check_duration_seconds | service, check | Duration of check execution in seconds |

In addition some checkers could provide their own metrics the list of them is bellow:

#### tls_certificate check

| Metric name                    | Labels      | Description                                  |
|--------------------------------|-------------|----------------------------------------------|
| certificate_expires_in_seconds | check, path | Time the certificate expires in (in seconds) |

#### icmp_ping check

| Metric name                                      | Labels      | Description                                |
|--------------------------------------------------|-------------|--------------------------------------------|
| anycastd_check_avg_rtt_seconds                   | check, host | Avg RTT of ICMP checks                     |
| anycastd_check_loss_percent                      | check, host | Percent of packet loss                     |
| anycastd_check_max_rtt_seconds                   | check, host | Max RTT of ICMP checks                     |
| anycastd_check_min_rtt_seconds                   | check, host | Min RTT of ICMP checks                     |
| anycastd_check_packets_received_duplicates_total | check, host | Total amount of duplicate packets received |
| anycastd_check_packets_received_total            | check, host | Total amount of packets received           |
| anycastd_check_packets_sent_total                | check, host | Total amount of packets sent               |
| anycastd_check_std_dev_rtt_seconds               | check, host | Standard deviation RTT of ICMP checks      |

### GoBGP

The core of anycastd for BGP communication is GoBGP which allows so gather
some details about peers, sessions and announces.

| Metric name                                      | Labels          | Description                                                                                         |
|--------------------------------------------------|-----------------|-----------------------------------------------------------------------------------------------------|
| anycastd_gobgp_peer_admin_state                  | router_id, peer | Peer state 0=up, 1=down, 2=pfx_ct                                                                   |
| anycastd_gobgp_peer_count                        | router_id       | Total amount of peers configured for the GoBGP instance                                             |
| anycastd_gobgp_peer_flops_count                  | router_id, peer | Peer flops count                                                                                    |
| anycastd_gobgp_peer_out_queue_count              | router_id, peer | Peer outgoing messages queue                                                                        |
| anycastd_gobgp_peer_password_set_flag            | router_id, peer | Whether the peer have peer password set flag set                                                    |
| anycastd_gobgp_peer_remove_private_flag          | router_id, peer | Whether the peer have remove private flag set                                                       |
| anycastd_gobgp_peer_send_community_flag          | router_id, peer | Whether the peer have send community flag set                                                       |
| anycastd_gobgp_peer_session_state                | router_id, peer | Peer session state 0=unknown, 1=idle, 2=connect, 3=active, 4=opensent, 5=openconfirm, 6=established |
| anycastd_gobgp_peer_type                         | router_id, peer | Peer type 0=internal, 1=external                                                                    |
| anycastd_gobgp_received_messages_keepalive       | router_id, peer | Number of Keepalive messages received from the peer                                                 |
| anycastd_gobgp_received_messages_notification    | router_id, peer | Number of Notification messages received from the peer                                              |
| anycastd_gobgp_received_messages_open            | router_id, peer | Number of Open messages received from the peer                                                      |
| anycastd_gobgp_received_messages_refresh         | router_id, peer | Number of Refresh messages received from the peer                                                   |
| anycastd_gobgp_received_messages_total           | router_id, peer | Total number of messages received from the peer                                                     |
| anycastd_gobgp_received_messages_update          | router_id, peer | Number of Update messages received from the peer                                                    |
| anycastd_gobgp_received_messages_withdraw_update | router_id, peer | Number of Withdraw Update messages received from the peer                                           |
| anycastd_gobgp_sent_messages_keepalive           | router_id, peer | Number of Keepalive messages sent to the peer                                                       |
| anycastd_gobgp_sent_messages_notification        | router_id, peer | Number of Notification messages sent to the peer                                                    |
| anycastd_gobgp_sent_messages_open                | router_id, peer | Number of Open messages sent to the peer                                                            |
| anycastd_gobgp_sent_messages_refresh             | router_id, peer | Number of Refresh messages sent to the peer                                                         |
| anycastd_gobgp_sent_messages_total               | router_id, peer | Total number of messages sent to the peer                                                           |
| anycastd_gobgp_sent_messages_update              | router_id, peer | Number of Update messages sent to the peer                                                          |
| anycastd_gobgp_sent_messages_withdraw_prefix     | router_id, peer | Number of Withdraw Prefix messages sent to the peer                                                 |
| anycastd_gobgp_sent_messages_withdraw_update     | router_id, peer | Number of Withdraw Update messages sent to the peer                                                 |
