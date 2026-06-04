# chtiwt

A minimal Twitch-style live streaming platform: RTMP ingest, HLS playback, and real-time chat. Written in Go, intentionally scoped down to the parts that show how live video actually moves through a system.

## Stack

| Layer | Choice |
|---|---|
| Language | Go 1.25 |
| HTTP / WebSocket | `net/http` (1.22+ routing), `coder/websocket` |
| Database | PostgreSQL 16 via `jackc/pgx/v5` + `sqlc` |
| Migrations | `golang-migrate` (embedded into the binary) |
| RTMP ingest | `yutopp/go-rtmp` + `yutopp/go-flv` |
| HLS muxing | `ffmpeg` (subprocess) |
| Auth | bcrypt + HttpOnly session cookies |
| Templating | `html/template` |
| Container | Multi-stage Dockerfile (debian-slim runtime) |

## Architecture

```
   OBS ──RTMP/1935──▶ chtiwt ─FLV→pipe→ ffmpeg ─▶ ./state/<channel>/index.m3u8
                       │                              │
                       │                              ▼
                       │                       Browser <video> via hls.js
                       │                              (HTTP /hls/<channel>/...)
                       │
                       └──WS /ws/chat/{channel}──▶ Room actor ──▶ broadcast
                                                  (presence, viewer count,
                                                   per-user rate limit)
```

In-memory `LiveRegistry` is the sole source of truth for who is currently live; the database keeps a historical `stream_sessions` table that `RecoverOnBoot` reconciles after a crash. Chat is a per-channel actor goroutine with channel-driven join/leave/send — no locks inside the room.

## Quick start

Requires Docker (with Compose v2). No other tooling needed for the run path.

```sh
docker compose up --build
```

Then:
- **Watch UI**: <http://localhost:8080>
- **Sign up** at `/signup`. Your stream key is shown exactly once on the dashboard immediately after.
- **Stream from OBS**:
  - *Settings → Stream → Service*: Custom
  - *Server*: `rtmp://localhost:1935/live`
  - *Stream Key*: paste the `live_…` value from your dashboard
  - Click Start Streaming. Within ~4 seconds your channel appears on the home page.
- **Chat**: open `/c/<your-channel>` in a second window (or as another user) to watch and talk.

Tear down with `docker compose down`. Add `-v` to wipe the Postgres data volume.

## Local development (without Docker)

You still need PostgreSQL and ffmpeg on `$PATH`.

```sh
# bring up just postgres
docker compose up -d postgres

# install dev tooling
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# generate query code (optional — checked-in copies usually suffice)
make generate

# run; migrations are applied automatically on boot
make run
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres connection string |
| `HTTP_ADDR` | `:8080` | HTTP listener |
| `RTMP_ADDR` | `:1935` | RTMP ingest listener |
| `STATE_DIR` | `./state` | Where HLS segments are written |
| `FFMPEG_PATH` | `ffmpeg` | Binary used as the HLS muxer subprocess |
| `SESSION_TTL` | `720h` (30 days) | Cookie + DB session lifetime |
| `SECURE_COOKIES` | `false` | Set to `true` behind HTTPS |
| `PUBLIC_RTMP_URL` | `rtmp://localhost:1935/live` | Shown on the dashboard as the OBS server URL |

## Status

Implemented:

- Auth: signup, login, sessions, stream key rotation
- RTMP ingest with stream-key auth
- HLS output via ffmpeg subprocess; single `tearDown` for every termination path; recovery sweep on boot
- Watch page with hls.js
- WebSocket chat: per-channel actor, presence broadcasts, per-user rate limits, viewer counts with cookie-based guest dedup
- Embedded migrations, single-command Docker deployment, CI

Not yet:

- Chat message persistence
- Channel pages / follow system / VODs
- Thumbnails on feed cards
- HTTPS / reverse proxy (deploy-target specific)

## License

This is a portfolio project — feel free to read, fork, and learn from it.
