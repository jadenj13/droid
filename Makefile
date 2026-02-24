.PHONY: build run run-planner run-executor run-reviewer \
        docker-build docker-up docker-down docker-logs \
        test lint clean

# ── Local ─────────────────────────────────────────────────────────────────────

build:
	go build -o bin/ ./cmd/...

run-planner: build
	./bin/planner

run-executor: build
	./bin/executor

run-reviewer: build
	./bin/reviewer

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin/

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-restart:
	docker compose restart

# Run a single service (usage: make docker-service SERVICE=executor)
docker-service:
	docker compose up -d $(SERVICE)
