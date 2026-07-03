.PHONY: run prod docs

run:
	go run cmd/task-management/main.go

prod:
	APP_ENV=production go run cmd/task-management/main.go

docs:	
	swag init -g cmd/task-management/main.go -o docs/ --parseDependency --parseInternal