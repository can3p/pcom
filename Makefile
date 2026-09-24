.PHONY: shell tunnel lint test build check fix check-q test-q vet-q cover-q

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
	go test ./...

build:
	go build -v ./...

check:
	go build -o /dev/null ./...
	go test ./...

fix:
	go fix ./...

# Quiet variants for agents: one line on success, a trimmed report on failure
# (full output goes to a log file). `make check` stays the verbose CI form.
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
