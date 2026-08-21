// Package integration exercises the repositories against a real MySQL and
// Redis instance and is deliberately excluded from the default build.
//
// Every file in this package carries the "integration" build tag so that
// plain `go test ./...` stays fast and offline.
//
// Run the suite:
//
//	docker compose -f docker-compose.test.yml up -d --wait
//	go test -tags integration -race ./internal/test/... -count=1 -v
//
// Or simply: `make integration`.
package integration
