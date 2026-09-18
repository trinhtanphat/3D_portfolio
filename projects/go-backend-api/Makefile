.PHONY: test run fmt vet up down

test:
	go test ./...

run:
	go run ./cmd/api

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

up:
	docker compose up --build

down:
	docker compose down -v
