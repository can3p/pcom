.PHONY: shell tunnel lint test build

shell:
	flyctl postgres connect -a pcomdb

tunnel:
	flyctl proxy 5432 -a pcomdb

pprof:
	flyctl proxy 8081 -a pcomdb

lint:
	golangci-lint run ./... --timeout=5m

test:
	go test ./...

build:
	go build -v ./...
