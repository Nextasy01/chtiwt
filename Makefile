# chtiwt local dev targets.
#
# Production deploys use `docker compose up` — the binary runs migrations on
# boot via its embedded copy of internal/store/migrations/. The `migrate`
# CLI is only needed for `make migrate-down` (rolling back during dev).
#
# Required tooling for dev:
#   go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
#   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest  # optional: only for migrate-down
#   ffmpeg on $PATH (any recent build, used as a subprocess for HLS muxing)

POSTGRES_URL ?= postgres://chtiwt:chtiwt@localhost:5432/chtiwt?sslmode=disable
HTTP_ADDR    ?= :8080
RTMP_ADDR    ?= :1935
STATE_DIR    ?= ./state

.PHONY: up down logs migrate migrate-down generate run build test tidy

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f postgres

migrate:
	migrate -path internal/store/migrations -database "$(POSTGRES_URL)" up

migrate-down:
	migrate -path internal/store/migrations -database "$(POSTGRES_URL)" down 1

generate:
	sqlc generate

run: export DATABASE_URL := $(POSTGRES_URL)
run: export HTTP_ADDR    := $(HTTP_ADDR)
run: export RTMP_ADDR    := $(RTMP_ADDR)
run: export STATE_DIR    := $(STATE_DIR)
run:
	go run ./cmd/chtiwt

build:
	go build -o chtiwt ./cmd/chtiwt

test:
	go test ./...

tidy:
	go mod tidy
