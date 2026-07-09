.PHONY: build run test test-race lint fmt clean docker-build docker-up docker-down docker-logs compose-up staging-up staging-down staging-logs staging-compose-up deps

BINARY=bin/server

build:
	go build -o $(BINARY) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v ./... -count=1

test-race:
	go test -race ./... -count=1

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; running go vet"; go vet ./...; \
	fi

fmt:
	gofmt -s -w .
	go mod tidy

clean:
	rm -rf bin/ coverage.out coverage.html

docker-build:
	docker compose --env-file stack.env build

docker-up:
	docker compose --env-file stack.env up -d

docker-down:
	docker compose --env-file stack.env down

docker-logs:
	docker compose --env-file stack.env logs -f

compose-up:
	docker compose --env-file stack.env up -d --build

staging-up:
	docker compose -f docker-compose.staging.yml --env-file stack.env up -d

staging-down:
	docker compose -f docker-compose.staging.yml --env-file stack.env down

staging-logs:
	docker compose -f docker-compose.staging.yml --env-file stack.env logs -f

staging-compose-up:
	docker compose -f docker-compose.staging.yml --env-file stack.env up -d --build

deps:
	go mod download
	go mod tidy
