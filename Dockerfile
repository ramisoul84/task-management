# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

# Dependencies change rarely: cache them in their own layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
      -o /out/task-management ./cmd/task-management

# ---- Runtime stage ----
# distroless: no shell or package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/task-management /app/task-management
COPY migrations /app/migrations

ENV APP_ENV=production \
    MIGRATIONS_DIR=/app/migrations \
    HTTP_PORT=8080

EXPOSE 8080
USER nonroot:nonroot

# No curl in distroless: the binary probes its own /livez.
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=3 \
  CMD ["/app/task-management", "healthcheck"]

ENTRYPOINT ["/app/task-management"]
CMD ["serve"]
