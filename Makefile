SHELL := /bin/sh

GO ?= go
SWAG ?= swag
DOCKER_COMPOSE ?= docker compose
ENV_FILE ?= .env.dev

.DEFAULT_GOAL := help

.PHONY: help env-check services-up docker-up docker-down docker-logs migrate swag swag-install run run-no-bot local test vet

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

env-check:
	@test -f "$(ENV_FILE)" || { \
		echo "$(ENV_FILE) not found. Create it with: cp .env.example $(ENV_FILE)"; \
		exit 1; \
	}

services-up: env-check ## Start and wait for local Homebrew PostgreSQL and Redis
	@command -v brew >/dev/null 2>&1 || { echo "Homebrew is required for local services"; exit 1; }
	@brew services start postgresql@18 >/dev/null
	@brew services start redis >/dev/null
	@set -a; . ./$(ENV_FILE); set +a; \
		until pg_isready -h "$$DB_HOST" -p "$$DB_PORT" -U "$$DB_USER" -d "$$DB_NAME" >/dev/null 2>&1; do sleep 1; done; \
		until redis-cli -u "$$REDIS_URL" ping >/dev/null 2>&1; do sleep 1; done

docker-up: env-check ## Start local PostgreSQL and Redis
	$(DOCKER_COMPOSE) --env-file $(ENV_FILE) up -d postgres redis
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) exec -T postgres \
		sh -c 'until pg_isready -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" >/dev/null 2>&1; do sleep 1; done'
	@$(DOCKER_COMPOSE) --env-file $(ENV_FILE) exec -T redis \
		sh -c 'until redis-cli ping >/dev/null 2>&1; do sleep 1; done'

docker-down: env-check ## Stop local Docker services
	$(DOCKER_COMPOSE) --env-file $(ENV_FILE) down

docker-logs: env-check ## Follow PostgreSQL and Redis logs
	$(DOCKER_COMPOSE) --env-file $(ENV_FILE) logs -f postgres redis

migrate: services-up ## Apply migrations to local PostgreSQL
	@set -e; \
	set -a; . ./$(ENV_FILE); set +a; \
	for migration in migrations/*.up.sql; do \
		echo "Applying $$migration"; \
		PGPASSWORD="$$DB_PASSWORD" psql -v ON_ERROR_STOP=1 \
			-h "$$DB_HOST" -p "$$DB_PORT" -U "$$DB_USER" -d "$$DB_NAME" < "$$migration"; \
	done

swag: ## Generate Swagger files under docs/v1
	@command -v $(SWAG) >/dev/null 2>&1 || { \
		echo "swag is not installed. Run: make swag-install"; \
		exit 1; \
	}
	$(SWAG) init -g cmd/api/main.go -o docs/v1

swag-install: ## Install the Swagger generator
	$(GO) install github.com/swaggo/swag/cmd/swag@latest

run: services-up swag ## Refresh Swagger and run API with local services
	$(GO) run ./cmd/api

run-no-bot: services-up swag ## Run locally without Telegram polling
	TELEGRAM_BOT_TOKEN= $(GO) run ./cmd/api

local: migrate run ## Start dependencies, migrate, generate Swagger, and run API

test: ## Run all Go tests
	$(GO) test ./...

vet: ## Run Go static checks
	$(GO) vet ./...
