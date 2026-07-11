# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/task-management ./cmd/task-management

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy binary
COPY --from=builder /out/task-management /app/task-management

# Copy migrations
COPY --from=builder /src/migrations /app/migrations

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/task-management"]