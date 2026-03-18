.PHONY: build build-editor build-go test test-race test-coverage lint fmt dev clean migrate-up migrate-down \
	bench bench-expr bench-engine bench-config bench-plugins bench-registry bench-save bench-compare \
	loadtest loadtest-baseline

build: build-editor build-go

build-editor:
	cd editor && npm ci --silent && npm run build
	rm -rf editorfs/dist
	cp -r editor/dist editorfs/dist

build-go:
	go build -tags embed_editor -ldflags "\
		-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.1-dev) \
		-X main.Commit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
		-X main.BuildTime=$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-o dist/noda ./cmd/noda

test:
	go test ./... -race -count=1

test-race:
	go test -race -count=1 ./internal/engine/ ./internal/connmgr/ ./internal/wasm/ ./internal/worker/ ./internal/scheduler/

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
	go run ./cmd/noda migrate up

migrate-down:
	go run ./cmd/noda migrate down

# Benchmarks
BENCH_COUNT ?= 3
BENCH_DIR ?= benchmarks

bench:
	go test ./... -bench=. -benchmem -count=$(BENCH_COUNT) -run='^$$' -timeout=10m

bench-expr:
	go test ./internal/expr/ -bench=. -benchmem -count=5 -run='^$$'

bench-engine:
	go test ./internal/engine/ -bench=. -benchmem -count=5 -run='^$$'

bench-config:
	go test ./internal/config/ -bench=. -benchmem -count=5 -run='^$$'

bench-plugins:
	go test ./plugins/core/transform/ ./plugins/core/control/ ./plugins/core/response/ -bench=. -benchmem -count=5 -run='^$$'

bench-registry:
	go test ./internal/registry/ -bench=. -benchmem -count=5 -run='^$$'

bench-save:
	@mkdir -p $(BENCH_DIR)
	go test ./... -bench=. -benchmem -count=$(BENCH_COUNT) -run='^$$' -timeout=10m \
		| tee $(BENCH_DIR)/bench-$$(date +%Y%m%d-%H%M%S).txt

bench-compare:
	@if [ -z "$(OLD)" ] || [ -z "$(NEW)" ]; then \
		echo "Usage: make bench-compare OLD=path/to/old.txt NEW=path/to/new.txt"; \
		exit 1; \
	fi
	benchstat $(OLD) $(NEW)

# Load testing (requires k6: https://k6.io)
LOADTEST_BASE_URL ?= http://localhost:3000

loadtest:
	k6 run --env BASE_URL=$(LOADTEST_BASE_URL) loadtest/scenarios/baseline.js
	k6 run --env BASE_URL=$(LOADTEST_BASE_URL) loadtest/scenarios/workflow-chain.js
	k6 run --env BASE_URL=$(LOADTEST_BASE_URL) loadtest/scenarios/concurrent.js

loadtest-baseline:
	k6 run --env BASE_URL=$(LOADTEST_BASE_URL) loadtest/scenarios/baseline.js
