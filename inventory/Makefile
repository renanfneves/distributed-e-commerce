SHELL := /bin/sh

GO ?= go
GOOSE_VERSION ?= v3.28.0

MODULE_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
TOOLS_DIR := $(MODULE_DIR)/.bin
GOOSE := $(TOOLS_DIR)/goose
ENV_FILE := $(MODULE_DIR)/.env
MIGRATIONS_DIR := $(MODULE_DIR)/internal/database/migrations
DOTENV := $(GO) run github.com/joho/godotenv/cmd/godotenv

.PHONY: test test-race migrate-test check-test-db vet check tools

test: migrate-test
	@cd "$(MODULE_DIR)" && \
		$(DOTENV) -f "$(ENV_FILE)" \
		$(GO) test ./... -v -count=1

test-race: migrate-test
	@cd "$(MODULE_DIR)" && \
		$(DOTENV) -f "$(ENV_FILE)" \
		$(GO) test -race ./... -count=1

migrate-test: check-test-db $(GOOSE)
	@$(DOTENV) -f "$(ENV_FILE)" sh -eu -c '\
		"$(GOOSE)" -dir "$(MIGRATIONS_DIR)" \
		postgres "$$TEST_DATABASE_URL" up'

check-test-db:
	@$(DOTENV) -f "$(ENV_FILE)" sh -eu -c '\
		database_url="$${TEST_DATABASE_URL:-}"; \
		if [ -z "$$database_url" ]; then \
			echo "erro: TEST_DATABASE_URL não está definida" >&2; \
			exit 1; \
		fi; \
		database_name="$${database_url%%\?*}"; \
		database_name="$${database_name##*/}"; \
		case "$$database_name" in \
			*_test) ;; \
			*) \
				echo "erro: o banco de teste deve terminar em _test; recebido $$database_name" >&2; \
				exit 1 ;; \
		esac'

vet:
	@cd "$(MODULE_DIR)" && $(GO) vet ./...

check: vet test

tools: $(GOOSE)

$(GOOSE):
	@mkdir -p "$(TOOLS_DIR)"
	GOBIN="$(TOOLS_DIR)" $(GO) install \
		github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
