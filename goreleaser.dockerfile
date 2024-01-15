FROM scratch

COPY schema-server /app/
COPY schemac /app/
WORKDIR /app

ENTRYPOINT [ "/app/schema-server" ]
