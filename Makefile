.PHONY: run prod docs test race cover test-integration

run:
	go run cmd/task-management/main.go


docs:	
	swag init -g cmd/task-management/main.go -o docs/ --parseDependency --parseInternal

test:
	go test -v ./...

race:
	go test -race ./...

cover:
	go test ./internal/service/ -cover

test-cover-html:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Requires a running MySQL with migrations applied. See comment at the top of
# internal/repository/team_stats_integration_test.go for setup.
test-integration:
	go test -tags=integration ./internal/repository/... -run TestGetTeamStats -v