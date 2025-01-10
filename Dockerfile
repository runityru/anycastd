FROM alpine:3.20.3 AS certificates

RUN apk add --update --no-cache \
  ca-certificates=20241121-r1

FROM scratch

COPY --from=certificates /etc/ssl/cert.pem /etc/ssl/cert.pem
COPY --chmod=0755 --chown=root:root dist/anycastd_linux_amd64_v3/anycastd /anycastd

ENTRYPOINT [ "/anycastd" ]
