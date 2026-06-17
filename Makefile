.PHONY: run prod

run:
	go run cmd/task-management/main.go

prod:
	APP_ENV=production go run cmd/task-management/main.go