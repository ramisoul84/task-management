# Task Management Service

REST API for team-based task management with role-based access control, built with Go.

## Live Demo

🚀 **Test the API live**: [Task Management Site](https://ramisoul84.github.io/task-management-site/)

The frontend is deployed on GitHub Pages and connects to this API at `https://ramisuliman.com/tm-api`.

## Features

- JWT auth with refresh token rotation
- RBAC (owner, admin, member)
- Task CRUD with optimistic locking
- Change history and comments
- Team analytics
- Redis caching (5 min TTL)
- Rate limiting
- Swagger docs

## Tech Stack

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![MySQL](https://img.shields.io/badge/MySQL-8.0-4479A1?style=flat&logo=mysql)
![Redis](https://img.shields.io/badge/Redis-7.0-DC382D?style=flat&logo=redis)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker)
![Fiber](https://img.shields.io/badge/Fiber-2.x-00ADD8?style=flat&logo=go)
![Swagger](https://img.shields.io/badge/Swagger-OpenAPI-85EA2D?style=flat&logo=swagger)

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Or: Go 1.26+, MySQL 8, Redis 7

### Docker Setup (Recommended)

```bash
# 1. Clone repository
git clone https://github.com/ramisoul84/task-management.git
cd task-management

# 2. Configure environment
cp .env.example .env
# Edit .env and set:
#   - SECRET (JWT signing secret)
#   - DB_USER, DB_PASSWORD, DB_NAME
#   - REDIS_PASSWORD

# 3. Start services
docker compose up -d --wait

# 4. Access
# API:        http://localhost:8080
# Swagger UI: http://localhost:8080/docs
# Health:     http://localhost:8080/health
```

## API Endpoints

Base URL: `/api/v1`  
Auth: `Authorization: Bearer <token>` (except `/auth/*`)

### Authentication

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/register` | Register new user | Public |
| POST | `/login` | Login and get tokens | Public |
| POST | `/auth/refresh` | Refresh access token | Refresh cookie |
| POST | `/auth/logout` | Logout and revoke tokens | Refresh cookie |

`/auth/register` and `/auth/login` remain available as backwards-compatible aliases.

### Teams

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/teams` | Create team | Authenticated |
| GET | `/teams` | List my teams | Authenticated |
| POST | `/teams/:id/invite` | Invite member | Owner, Admin |
| GET | `/teams/:id/members` | List members | Team member |
| PATCH | `/teams/:id/members/:uid/role` | Change member role | Owner, Admin |
| DELETE | `/teams/:id/members/:uid` | Remove member | Owner, Admin |
| GET | `/teams/:id/stats` | Team analytics | Owner, Admin |

### Tasks

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/tasks` | Create task | Team member |
| GET | `/tasks` | List tasks (filters, pagination) | Team member |
| PUT | `/tasks/:id` | Update task | Creator, Admin, Owner |
| GET | `/tasks/:id/history` | Task change history | Team member |
| GET | `/tasks/:id/comments` | List comments | Team member |
| POST | `/tasks/:id/comments` | Add comment | Team member |

## SQL report integration test

The repository includes an opt-in MySQL integration test for the required team
statistics query. Start the stack, then run:

```bash
INTEGRATION_MYSQL_DSN='task_management:change-me@tcp(localhost:3306)/task_management?parseTime=true' \
  go test -tags=integration ./internal/repository
```
