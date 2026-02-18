.PHONY: shell tunnel lint test build check fix

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
