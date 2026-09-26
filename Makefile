.PHONY: shell tunnel lint test test-short cover build check fix check-q test-q vet-q cover-q model

PKG ?= ./...

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
	@GOCOVERDIR=$(COVDIR) go test -cover ./... -args -test.gocoverdir=$(COVDIR)
	@go tool covdata percent -i=$(COVDIR) | perl -pe 's/\t\t\t/\n/g' | grep "coverage:" | grep -v github.com/can3p/pcom/pkg/model/core
	@go tool covdata textfmt -i=$(COVDIR) -o coverage.out

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
# Narrow with PKG, for example `make test-q PKG=./pkg/links/...`.
check-q:
	@tools/qrun.sh build go build -o /dev/null ./...
	@tools/qrun.sh vet go vet ./...
	@tools/qrun.sh test go test ./...

test-q:
	@tools/qrun.sh test go test $(PKG)

vet-q:
	@tools/qrun.sh vet go vet $(PKG)

cover-q:
	@QRUN_SHOW_OK=1 tools/qrun.sh cover go test -cover $(PKG)

# Shape of a generated model without reading pkg/model/core:
# `make model` lists the models, `make model T=User` prints one.
model:
	@tools/model.sh $(T)
