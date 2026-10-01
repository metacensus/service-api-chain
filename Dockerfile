# syntax=docker/dockerfile:1

# Build natively and cross-compile: no QEMU per target architecture.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/service

# No ca-certificates: the peer is verified against FABRIC_PEER_TLS_CA alone.
FROM alpine:3.23 AS runner
ENV PORT=3001

COPY --from=builder /out/service /app/service

RUN addgroup -S app && adduser -S -G app app
USER app

EXPOSE ${PORT}

# BusyBox wget is in the base.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1

ENTRYPOINT ["/app/service"]
