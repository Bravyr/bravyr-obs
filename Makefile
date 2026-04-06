.PHONY: lint test test-race vet fmt check swagger clean

GOLANGCI_LINT := golangci-lint

lint:
	$(GOLANGCI_LINT) run ./...

test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -w .
	goimports -w .

check: fmt vet lint test

swagger:
	@echo "No swagger spec for library package"

clean:
	go clean -testcache
