FROM ghcr.io/blinklabs-io/go:1.24.7-1 AS build

WORKDIR /code
RUN go env -w GOCACHE=/go-cache
RUN go env -w GOMODCACHE=/gomod-cache
COPY go.* .
RUN --mount=type=cache,target=/gomod-cache go mod download
COPY . .
RUN --mount=type=cache,target=/gomod-cache --mount=type=cache,target=/go-cache make build

FROM cgr.dev/chainguard/glibc-dynamic AS vpn-indexer
COPY --from=build /code/vpn-indexer /bin/
ENTRYPOINT ["vpn-indexer"]
