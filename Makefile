.PHONY: build test test-coverage lint fmt dev clean migrate-up migrate-down

build:
	go build -o dist/noda ./cmd/noda

test:
	go test ./... -race -count=1

test-coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

dev:
	docker compose up --build

clean:
	rm -rf dist/ coverage.out coverage.html

migrate-up:
	@echo "not yet implemented"

migrate-down:
	@echo "not yet implemented"
