FROM scratch

COPY --chmod=0755 --chown=root:root dist/anycastd_linux_amd64_v3/anycastd /anycastd

ENTRYPOINT [ "/anycastd" ]
