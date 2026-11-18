.PHONY: env run dev test docker up build

env:
	@test -f .env || cp .env.example .env

run: env
	set -a && . ./.env && set +a && go run ./cmd/attendance

dev: env
	set -a && . ./.env && set +a && air

test:
	go test ./...

build:
	mkdir -p bin
	go build -o ./bin/attendance ./cmd/attendance

docker:
	docker build -t attendance .

up:
	docker compose up --build
