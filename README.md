# anycastd

[![Verify](https://github.com/teran/anycastd/actions/workflows/verify.yml/badge.svg?branch=master)](https://github.com/teran/anycastd/actions/workflows/verify.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/teran/anycastd)](https://goreportcard.com/report/github.com/teran/anycastd)
[![Go Reference](https://pkg.go.dev/badge/github.com/teran/anycastd.svg)](https://pkg.go.dev/github.com/teran/anycastd)

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
      - kind: http_2xx
        spec:
          address: 127.0.0.1:8080
          path: /
          tries: 3
          interval: 100ms
          timeout: 2s
      - kind: assigned_address
        spec:
          interface: dummy0
          ipv4: 33.22.11.0
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
