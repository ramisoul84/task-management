APP        := task-management
MAIN       := ./cmd/task-management
BIN_DIR    := bin
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildTime=$(BUILD_TIME)

GOLANGCI_VERSION := v2.14.0
GOLANGCI         := $(shell go env GOPATH)/bin/golangci-lint

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## ---------- Development ----------

.PHONY: run
run: ## Run the API locally (loads .env.dev)
	go run $(MAIN)

.PHONY: build
build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP) $(MAIN)

.PHONY: tidy
tidy: ## Tidy and verify go modules
	go mod tidy
	go mod verify

.PHONY: fmt
fmt: ## Format code
	gofmt -s -w .
	go run golang.org/x/tools/cmd/goimports@latest -local github.com/ramisoul84/task-management -w .

.PHONY: lint
lint: $(GOLANGCI) ## Run golangci-lint
	$(GOLANGCI) run ./...

$(GOLANGCI):
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

.PHONY: vet
vet: ## Run go vet
	go vet ./...

## ---------- Tests ----------

.PHONY: test
test: ## Run unit tests
	go test ./... -count=1

.PHONY: race
race: ## Run unit tests with the race detector
	go test -race ./... -count=1

.PHONY: test-integration
test-integration: ## Run integration tests (spins up MySQL/Redis via testcontainers; needs Docker)
	go test -tags=integration ./... -count=1 -v -run Integration

.PHONY: cover
cover: ## Generate an HTML coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

## ---------- Docs ----------

.PHONY: docs
docs: ## Regenerate Swagger/OpenAPI docs
	swag fmt -g cmd/task-management/main.go
	swag init -g cmd/task-management/main.go -o docs/ --parseDependency --parseInternal

## ---------- Docker ----------

.PHONY: up
up: ## Start the full stack (app, MySQL, Redis)
	docker compose up -d --build --wait

.PHONY: down
down: ## Stop the stack
	docker compose down

.PHONY: logs
logs: ## Tail application logs
	docker compose logs -f app

.PHONY: infra
infra: ## Start only MySQL and Redis (for `make run`)
	docker compose up -d --wait mysql redis

## ---------- Database ----------

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations
	go run $(MAIN) migrate up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	go run $(MAIN) migrate down

.PHONY: migration
migration: ## Create a new migration pair: make migration name=add_x
	@test -n "$(name)" || (echo "usage: make migration name=<name>"; exit 1)
	@n=$$(ls migrations/*.up.sql | wc -l); n=$$(printf "%03d" $$((n+1))); \
		touch migrations/$${n}_$(name).up.sql migrations/$${n}_$(name).down.sql; \
		echo "created migrations/$${n}_$(name).{up,down}.sql"
