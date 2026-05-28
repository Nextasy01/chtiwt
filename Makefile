# chtiwt local dev targets.
# Required tooling (install once):
#   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
#   go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
# NOTE: the -tags 'postgres' is mandatory — migrate ships with no drivers by
# default, so omitting it produces: "unknown driver postgres (forgotten import?)".

POSTGRES_URL ?= postgres://chtiwt:chtiwt@localhost:5432/chtiwt?sslmode=disable
HTTP_ADDR    ?= :8080

.PHONY: up down logs migrate migrate-down generate run build test tidy

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f postgres

migrate:
	migrate -path migrations -database "$(POSTGRES_URL)" up

migrate-down:
	migrate -path migrations -database "$(POSTGRES_URL)" down 1

generate:
	sqlc generate

run: export DATABASE_URL := $(POSTGRES_URL)
run: export HTTP_ADDR    := $(HTTP_ADDR)
run:
	go run ./cmd/chtiwt

build:
	go build -o chtiwt ./cmd/chtiwt

test:
	go test ./...

tidy:
	go mod tidy
