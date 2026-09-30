.PHONY: build compose-config deps-up deps-down fmt migrate-down migrate-status migrate-up race run test verify vet

build:
	go build ./...

compose-config:
	docker compose config --quiet

deps-up:
	docker compose up -d --wait

deps-down:
	docker compose down

fmt:
	gofmt -w .

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

race:
	go test -race ./...

run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

verify: fmt vet test race build compose-config
