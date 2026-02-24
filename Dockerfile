# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o bin/planner  ./cmd/planner  && \
    go build -o bin/executor ./cmd/executor && \
    go build -o bin/reviewer ./cmd/reviewer

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/ ./bin/

# Default binary is overridden per-service in docker-compose.yml
CMD ["./bin/planner"]
