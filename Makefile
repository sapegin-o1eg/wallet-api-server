run:
	go run cmd/server/main.go

test:
	go test ./...

lint:
	golangci-lint run || true

fmt:
	gofmt -w .

.PHONY: run test lint fmt 