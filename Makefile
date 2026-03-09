.PHONY: build build-editor build-go test test-coverage lint fmt dev clean migrate-up migrate-down

build: build-editor build-go

build-editor:
	cd editor && npm ci --silent && npm run build
	rm -rf editorfs/dist
	cp -r editor/dist editorfs/dist

build-go:
	go build -tags embed_editor -o dist/noda ./cmd/noda

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
	rm -rf dist/ coverage.out coverage.html editorfs/dist/

migrate-up:
	@echo "not yet implemented"

migrate-down:
	@echo "not yet implemented"
