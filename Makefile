.PHONY: shell tunnel lint test test-short cover cover-check build check fix check-q test-q vet-q cover-q model ui-deps test-ui ui-trace screenshots \
	dev-up dev dev-logs dev-down migrate migrate-status migrate-down migration generate psql db-reset seed seed-reset \
	tools-shell migrate-prod

PKG ?= ./...
TAGS ?=
tags_flag = $(if $(TAGS),-tags $(TAGS))

shell:
	flyctl postgres connect -a pcomdb

tunnel:
	flyctl proxy 5433 -a pcomdb

pprof_tunnel:
	flyctl proxy 9090:8081 -a pcom

pprof_heap:
	go tool pprof -http localhost:9091 http://localhost:9090/debug/pprof/heap

lint:
	golangci-lint run ./... --timeout=5m

test:
	go test -coverprofile=coverage.out ./...

test-short:
	go test -short ./...

COVDIR := $(CURDIR)/.cover

cover:
	@rm -rf $(COVDIR)
	@mkdir -p $(COVDIR)
	@# -coverpkg: a test covers every pcom package it runs, not only its own, so
	@# code a service test reaches in pkg/repo counts (RS moved queries there).
	@GOCOVERDIR=$(COVDIR) go test -cover -coverpkg=$$(go list ./... | grep -v /pkg/model/core | paste -sd, -) ./... -args -test.gocoverdir=$(COVDIR)
	@go tool covdata percent -i=$(COVDIR) | perl -pe 's/\t\t\t/\n/g' | grep "coverage:" | grep -v github.com/can3p/pcom/pkg/model/core
	@go tool covdata textfmt -i=$(COVDIR) -o coverage.out

cover-check:
	@bash tools/coverage-check.sh

build:
	go build -v ./...

check:
	go build -o /dev/null ./...
	go vet ./...
	go test ./...

fix:
	go fix ./...

# Quiet variants for agents: one line on success, a trimmed report on failure
# (full output goes to a log file). They run the same steps as `make check`,
# which stays the verbose CI form.
# Narrow with PKG, for example `make test-q PKG=./pkg/links/...`, and pass
# build tags with TAGS, for example `make vet-q PKG=./e2e/browser/... TAGS=browser`.
# check-q also runs go fix and the linter first, as CI does: the Go Fix job
# commits whatever go fix rewrites, and the result must still pass Lint.
check-q: fix-q lint-q
	@tools/qrun.sh build go build -o /dev/null ./...
	@tools/qrun.sh vet go vet ./...
	@tools/qrun.sh test go test ./...

# go fix rewrites files in place; lint-q afterwards catches what a rewrite
# leaves behind (a helper that go fix inlined everywhere becomes unused).
fix-q:
	@tools/qrun.sh fix go fix $(tags_flag) $(or $(PKG),./...)

lint-q:
	@tools/qrun.sh lint golangci-lint run --timeout=5m $(if $(TAGS),--build-tags $(TAGS)) $(or $(PKG),./...)

test-q:
	@tools/qrun.sh test go test $(tags_flag) $(PKG)

vet-q:
	@tools/qrun.sh vet go vet $(tags_flag) $(PKG)

cover-q:
	@QRUN_SHOW_OK=1 tools/qrun.sh cover go test $(tags_flag) -cover $(PKG)

# Browser tests (e2e/browser, build tag `browser`). `make ui-deps` installs the
# Playwright driver and Chromium once (UI_DEPS_FLAGS=--with-deps also installs
# the system libraries, on Linux). `make test-ui` builds the frontend and runs
# the suite quietly: RUN=<regex> narrows it, COUNT=<n> repeats it, and
# HEADED=1 SLOWMO=250 shows the browser (headless otherwise). A failed test logs the path of its
# trace; open it with `make ui-trace F=<path>`.
PLAYWRIGHT = go run github.com/mxschmitt/playwright-go/cmd/playwright
RUN ?=
COUNT ?= 1
UI_DEPS_FLAGS ?=

ui-deps:
	$(PLAYWRIGHT) install $(UI_DEPS_FLAGS) chromium

test-ui:
	@tools/qrun.sh ui-build yarn --cwd cmd/web build
	@HEADED=$(HEADED) SLOWMO=$(SLOWMO) tools/qrun.sh test-ui go test -tags browser -count=$(COUNT) $(if $(RUN),-run '$(RUN)') ./e2e/browser/...

# `make screenshots` regenerates docs/guide/screenshots/ (build tag `screenshots`,
# never part of test-ui).
screenshots:
	@tools/qrun.sh ui-build yarn --cwd cmd/web build
	@HEADED=$(HEADED) SLOWMO=$(SLOWMO) tools/qrun.sh screenshots go test -tags browser,screenshots -count=1 ./e2e/browser/screenshots/

ui-trace:
	$(PLAYWRIGHT) show-trace $(F)

# Shape of a generated model without reading pkg/model/core:
# `make model` lists the models, `make model T=User` prints one.
model:
	@tools/model.sh $(T)

# Local stack (docker-compose.yml): Postgres on localhost:5442 and tommy (mail
# sink on http://localhost:8811/ui/, S3 on localhost:9555). The database
# targets run in the `tools` container against the compose database, as your
# UID/GID so the files they write belong to you.
#   make dev-up / dev-down          start or stop postgres and tommy
#   make migrate                    apply migrations (migrate-status, migrate-down)
#   make migration name=add_foo     create migrations/<timestamp>-add_foo.sql
#   make generate                   regenerate pkg/model/core from the migrations (throwaway DB)
#   make psql [ARGS="-c '...'"]     psql on the compose database
#   make db-reset                   drop, recreate and migrate the dev database
#   make seed / seed-reset          go run ./cmd/web seed [--reset]
#   make tools-shell [CMD='...']    bash in the tools container
COMPOSE ?= docker compose
export HOST_UID := $(shell id -u)
export HOST_GID := $(shell id -g)
TOOLS_RUN = $(COMPOSE) run --rm $(if $(shell [ -t 0 ] || echo notty),-T)
TOOLS = $(TOOLS_RUN) tools

dev-up:
	$(COMPOSE) up -d --wait postgres tommy
	@./tools/dev-urls.sh

# The app (live reload) and the frontend watcher in containers; foreground.
# The addresses are printed once the app answers.
dev:
	@./tools/dev-urls.sh --app & $(COMPOSE) --profile dev up

dev-logs:
	$(COMPOSE) --profile dev logs -f

dev-down:
	$(COMPOSE) --profile '*' down

migrate:
	$(TOOLS) ./sqlmigrate.sh up

migrate-status:
	$(TOOLS) ./sqlmigrate.sh status

migrate-down:
	$(TOOLS) ./sqlmigrate.sh down

migration:
	@test -n "$(name)" || { echo "usage: make migration name=add_foo" >&2; exit 1; }
	$(TOOLS) ./sqlmigrate.sh new $(name)

generate:
	$(TOOLS) ./generate.sh

psql:
	$(TOOLS) bash -c 'exec psql "$$DATABASE_URL" "$$@"' psql $(ARGS)

db-reset:
	$(COMPOSE) up -d --wait postgres
	$(COMPOSE) exec -T postgres sh -c 'dropdb -U "$$POSTGRES_USER" --if-exists --force "$$POSTGRES_DB" && createdb -U "$$POSTGRES_USER" "$$POSTGRES_DB"'
	$(MAKE) migrate

# Seeding builds cmd/web, which needs cgo and libvips (govips): only the dev
# image (service `app`) has them, the tools image does not.
seed:
	$(TOOLS_RUN) app go run . seed

seed-reset:
	$(TOOLS_RUN) app go run . seed --reset

tools-shell:
	$(TOOLS) bash $(if $(CMD),-c '$(CMD)')

# Production migrations through the fly proxy tunnel: run `make tunnel` in
# another terminal first. DATABASE_URL comes from ./env.pl (flyctl), pointed
# at the tunnel from inside the container. On Linux the tunnel must listen on
# an address the container can reach (flyctl proxy --bind-addr).
migrate-prod:
	@printf 'Apply migrations to the PRODUCTION database through the tunnel on :5433? [y/N] '; \
	 read ans; [ "$$ans" = y ] || { echo aborted; exit 1; }
	@DATABASE_URL=$$(./env.pl | sed -n 's/^DATABASE_URL=//p' | sed 's/@localhost:/@host.docker.internal:/'); \
	 test -n "$$DATABASE_URL" || { echo "env.pl printed no DATABASE_URL" >&2; exit 1; }; \
	 export DATABASE_URL; \
	 $(TOOLS_RUN) --no-deps -e DATABASE_URL -e PGSSLMODE=disable tools ./sqlmigrate.sh up -env=production
