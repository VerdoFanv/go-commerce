# syntax=docker/dockerfile:1

# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

WORKDIR /src
# install sertificate for secure
RUN apk add --no-cache ca-certificates

# install deps
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# nonactive c binding and result static binaries
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# ---------- Runtime base ----------
FROM alpine:3.20 AS runtime

WORKDIR /app
# add new user
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
