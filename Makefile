.PHONY: run prod build test race cover test-db integration

run:
	go run cmd/task-management/main.go

prod:
	APP_ENV=production go run cmd/task-management/main.go

build:
	go build -o bin/task-management cmd/task-management/main.go

test:
	go test -v ./...

race:
	go test -race ./internal/service/

cover:
	go test ./internal/service/ -cover

test-db:
	docker compose -f docker-compose.test.yml up -d --wait

integration: test-db
	go test -tags integration -race ./internal/test/... -count=1 -v