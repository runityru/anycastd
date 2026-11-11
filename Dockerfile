# anycastd container image.
#
# Multi-arch: goreleaser's dist/ artifact is copied to a plain root-level binary
# (anycastd-linux-amd64 / anycastd-linux-arm64) by `make container-image`, then
# mapped into the image per target architecture via ARG TARGETARCH.
# Minimal nonroot scratch image: TLS CA bundle and an /etc/passwd entry for the
# unprivileged user (UID 65534) are copied in from the alpine builder stage.
# Usage:
#   make build
#   make container-image push=true

FROM alpine:3.24.1 AS certificates

# hadolint ignore=DL3018
RUN apk add --update --no-cache \
  ca-certificates \
  && echo 'nobody:x:65534:65534:nobody:/:/sbin/nologin' > /etc/passwd-minimal

FROM scratch

ARG TARGETARCH

COPY --from=certificates /etc/ssl/cert.pem /etc/ssl/cert.pem
COPY --from=certificates /etc/passwd-minimal /etc/passwd

COPY anycastd-linux-${TARGETARCH} /anycastd

USER 65534:65534

ENTRYPOINT [ "/anycastd" ]

LABEL org.opencontainers.image.source="https://github.com/runityru/anycastd"
LABEL org.opencontainers.image.description="Go daemon that announces/withdraws anycast prefixes over BGP based on service health checks"
LABEL org.opencontainers.image.licenses="Apache-2.0"
