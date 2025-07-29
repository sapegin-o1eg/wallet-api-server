run:
	go run cmd/server/main.go

test:
	go test -count=1 ./...

test-integration:
	go test -count=1./tests/... -v

lint:
	golangci-lint run || true

fmt:
	gofmt -w .

.PHONY: run test test-integration lint fmt
