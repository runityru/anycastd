FROM alpine:3.22.1 AS certificates

RUN apk add --update --no-cache \
  ca-certificates=20250911-r0

FROM scratch

COPY --from=certificates /etc/ssl/cert.pem /etc/ssl/cert.pem
COPY --chmod=0755 --chown=root:root dist/anycastd_linux_amd64_v3/anycastd /anycastd

ENTRYPOINT [ "/anycastd" ]
