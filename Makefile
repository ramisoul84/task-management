.PHONY: run prod build test

run:
	go run cmd/task-management/main.go

prod:
	APP_ENV=production go run cmd/task-management/main.go

build:
	go build -o bin/task-management cmd/task-management/main.go

test:
	go test -v ./...