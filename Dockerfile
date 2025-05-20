FROM ghcr.io/blinklabs-io/go:1.24.2-1 AS build

WORKDIR /code
COPY go.* .
RUN go mod download
COPY . .
RUN make build

FROM cgr.dev/chainguard/glibc-dynamic AS vpn-indexer
COPY --from=build /code/vpn-indexer /bin/
ENTRYPOINT ["vpn-indexer"]
