.PHONY: fmt fmt-check lint mod-check test test-race check ci

fmt:
	gofmt -w .

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'Go files need formatting. Run make fmt:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

lint:
	golangci-lint run

mod-check:
	go mod tidy -diff

test:
	go test ./...

test-race:
	go test -race ./...

check: fmt-check lint mod-check test

ci: check test-race
