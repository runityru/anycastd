FROM alpine:3.24.1 AS certificates

# hadolint ignore=DL3018
RUN apk add --update --no-cache \
  ca-certificates

FROM scratch

COPY --from=certificates /etc/ssl/cert.pem /etc/ssl/cert.pem
COPY --chmod=0755 --chown=root:root dist/anycastd_linux_amd64_v3/anycastd /anycastd

ENTRYPOINT [ "/anycastd" ]
