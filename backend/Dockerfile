# syntax=docker/dockerfile:1.6
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/shophub ./...

FROM alpine:3.20 AS release
RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app
COPY --from=builder /out/shophub /app/shophub
RUN chown -R root:root /app && chmod 0755 /app/shophub
USER app
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["/app/shophub"]
