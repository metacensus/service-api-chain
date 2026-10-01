# syntax=docker/dockerfile:1

# Build natively and cross-compile: no QEMU per target architecture.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
WORKDIR /src
ARG TARGETOS TARGETARCH
COPY . .
# Downloads only what the binaries import, not the test-only graph go.mod also holds.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# No ca-certificates: it makes no outbound call.
FROM alpine:3.23 AS chaincode
COPY --from=builder /out/chaincode /app/chaincode
USER nobody
EXPOSE 9999
# No HEALTHCHECK: the peer dials this gRPC server, so readiness is its connection.
ENTRYPOINT ["/app/chaincode"]

# No ca-certificates: the peer is verified against FABRIC_PEER_TLS_CA alone.
FROM alpine:3.23 AS service
ENV PORT=3001
COPY --from=builder /out/service /app/service
USER nobody
EXPOSE ${PORT}
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1
ENTRYPOINT ["/app/service"]
