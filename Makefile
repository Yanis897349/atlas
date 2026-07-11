.PHONY: fmt fmt-check lint mod-check test test-race check ci db-up db-down db-reset

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

db-up:
	docker compose up --detach --wait postgres

db-down:
	docker compose down

db-reset:
	docker compose down --volumes --remove-orphans
	docker compose up --detach --wait postgres
