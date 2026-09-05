# syntax=docker/dockerfile:1

# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

# Layer-cache modules before source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static, stripped binaries: no CGO → no libc dependency, minimal attack surface.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# ---------- Runtime base ----------
# Alpine + non-root user + read-only-capable binary. For maximal hardening,
# swap this stage to gcr.io/distroless/static-debian12 (no shell at all).
FROM alpine:3.20 AS runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates wget \
	&& adduser -D -H -u 10001 golang

COPY --from=builder /out/api /app/api
COPY --from=builder /out/worker /app/worker
COPY docs /app/docs

USER golang

EXPOSE 8080

FROM runtime AS api
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8080/health/live || exit 1
CMD ["/app/api"]

FROM runtime AS worker
CMD ["/app/worker"]
