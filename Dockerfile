# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

FROM alpine:3.20 AS runtime

WORKDIR /app

RUN adduser -D -H -u 10001 golang

COPY --from=builder /out/api /app/api
COPY --from=builder /out/worker /app/worker
COPY docs /app/docs

USER golang

EXPOSE 8080

FROM runtime AS api
CMD ["/app/api"]

FROM runtime AS worker
CMD ["/app/worker"]
