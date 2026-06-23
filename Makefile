.PHONY: help install build run start test cover cover-html vet fmt tidy clean docker

BINARY := shortlink
COVERFILE := cover.out

# Runtime config — override on the command line, e.g.  make run PORT=9000
PORT ?= 8080
DB_PATH ?= shortlink.db
BASE_URL ?= http://localhost:$(PORT)
FEISTEL_KEY ?= shortlink-default-key
ENV := PORT=$(PORT) DB_PATH=$(DB_PATH) BASE_URL=$(BASE_URL) FEISTEL_KEY=$(FEISTEL_KEY)

help: ## Show this help (default target)
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

install: ## Download module dependencies
	go mod download
	go mod verify

build: ## Build a static CGO-free binary
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/server

run: ## Run from source (go run) with config above
	$(ENV) go run ./cmd/server

start: build ## Build the binary, then run it with config above
	$(ENV) ./$(BINARY)

test: ## Run all tests with the race detector
	go test -race ./...

cover: ## Print coverage (internal/* targets >=85%)
	go test -race -coverprofile=$(COVERFILE) ./...
	go tool cover -func=$(COVERFILE) | tail -n 1
	@echo "--- per package ---"
	@go test -cover ./internal/... 2>/dev/null | grep coverage

cover-html: cover ## Open an HTML coverage report
	go tool cover -html=$(COVERFILE)

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -s -w .

tidy: ## Tidy module dependencies
	go mod tidy

docker: ## Build the Docker image
	docker build -t $(BINARY) .

clean: ## Remove build/coverage artifacts and local DB files
	rm -f $(BINARY) $(COVERFILE) *.db *.db-shm *.db-wal

.DEFAULT_GOAL := help
