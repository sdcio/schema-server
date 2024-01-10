FROM golang:1.21.4 as builder

RUN apt-get update && apt-get install -y ca-certificates git-core ssh
RUN mkdir -p -m 0700 /root/.ssh && ssh-keyscan github.com >> /root/.ssh/known_hosts
RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/

RUN mkdir -p /build
COPY go.mod go.sum /build/

WORKDIR /build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    go mod download

ADD . /build
WORKDIR /build

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o schema-server .

FROM scratch
COPY --from=builder /build/schema-server /app/
WORKDIR /app
ENTRYPOINT [ "/app/schema-server" ]
