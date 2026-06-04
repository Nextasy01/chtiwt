# syntax=docker/dockerfile:1.7
# ---- builder ----------------------------------------------------------------
FROM golang:1.25-bookworm AS builder

WORKDIR /src

# Cache modules before pulling in source for better layer reuse.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Static, stripped, no debug info. CGO_ENABLED=0 means the binary doesn't
# need libc in the final image, but we still use debian:slim because ffmpeg
# wants a real userspace.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/chtiwt ./cmd/chtiwt

# ---- runtime ----------------------------------------------------------------
FROM debian:bookworm-slim

# - ffmpeg: required subprocess for RTMP→HLS muxing
# - ca-certificates: so hls.js + any future outbound HTTPS works
# - tini: PID 1 that reaps zombies and forwards SIGTERM to ffmpeg children
# - curl: used by the compose healthcheck
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates \
        tini \
        curl \
    && rm -rf /var/lib/apt/lists/*

# Non-root user. UID/GID 10001 stays out of common host UID ranges so a
# bind-mounted ./state dir on a dev box doesn't collide with a real user.
RUN groupadd --system --gid 10001 chtiwt \
    && useradd --system --uid 10001 --gid chtiwt --home-dir /app --shell /usr/sbin/nologin chtiwt

WORKDIR /app
COPY --from=builder /out/chtiwt /app/chtiwt

# /state holds HLS playlists + segments. RecoverOnBoot wipes it on every
# start so persistence isn't required, but the dir must exist and be
# writable by the chtiwt user.
RUN mkdir -p /state && chown chtiwt:chtiwt /state

USER chtiwt:chtiwt

ENV HTTP_ADDR=:8080 \
    RTMP_ADDR=:1935 \
    STATE_DIR=/state

EXPOSE 8080 1935

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/app/chtiwt"]
