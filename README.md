# anycastd

[![Verify](https://github.com/teran/anycastd/actions/workflows/verify.yml/badge.svg?branch=master)](https://github.com/teran/anycastd/actions/workflows/verify.yml)

anycastd is aimed to bring new age approaches to classic applications by allowing
to announce virtual addresses via BGP w/ service-level checks.

Let's imaging you gonna create your own DNS/HTTP/whatever else service so how
to utilize L3 load balancing and remove announce when something goes wrong on
particular node? anycastd is exactly for that! No bunch of dedicated services
and stateful control on BGP routes and no semi-tested Python scripts anymore!

## Configure

anycastd longs to follow [12factor](https://12factor.net) principles however
it's not always possible to encode complex data structures into environment
variable. So configuration is divided into two ways:

### Environment

anycastd allows to set the following options via environment variables:

* CONFIG_PATH (string, default: /config.yaml) - path to the configuration file
* LOG_LEVEL (ENUM, default: WARN) - logging verbosity level, could one of the:
  * TRACE
  * DEBUG
  * INFO
  * WARNING
  * ERROR
  * FATAL
  * PANIC

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
    check_operator: and
    check_interval: 10s
    checks:
      - kind: http_2xx
        spec:
          address: 127.0.0.1:8080
          path: /
          tries: 3
          interval: 100ms
          timeout: 2s
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

* http_2xx

Pending checks:

* dns_lookup - performs DNS lookup
* assigned_address - ensures the address is assigned on interface
