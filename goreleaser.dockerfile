FROM scratch

LABEL repo="https://github.com/iptecharch/schema-server"

COPY schema-server /app/
COPY schemac /app/
WORKDIR /app

ENTRYPOINT [ "/app/schema-server" ]
