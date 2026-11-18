.PHONY: env run dev test docker up

env:
	@test -f .env || cp .env.example .env

run: env
	set -a && . ./.env && set +a && go run ./cmd/attendance

dev: env
	set -a && . ./.env && set +a && air

test:
	go test ./...

docker:
	docker build -t attendance .

up:
	docker compose up --build
