# Atlas agent guide

## Project

Atlas is an open-source macro intelligence platform. It ingests financial news, economic events, government publications, and market data, then turns them into deterministic, source-cited context, narratives, briefs, and watchlists. Atlas supports analysis; it does not predict markets.

The project is at an early stage. Preserve source traceability, deterministic ingestion, and explicit domain boundaries as the architecture evolves.

## Pre-production database policy

Atlas is not live, and development and test databases are disposable. Prefer resetting the database and keeping schema changes simple over preserving old development data or supporting legacy schemas. Do not add compatibility fallbacks, schema baselining, transitional backfills, or other rollout machinery unless the user explicitly requires an upgrade path. This project-specific policy overrides general rollout-safety defaults until Atlas enters production.

## Required workflow

Use the Go and golangci-lint versions pinned by `.mise.toml`. The Go patch version is also pinned by the `toolchain` directive in `go.mod`.

After every change, including small fixes, run:

```sh
mise exec -- make fmt
mise exec -- make check
```

Before handing work off, run the complete CI-equivalent suite:

```sh
mise exec -- make ci
```

`make check` verifies formatting, runs golangci-lint, checks module tidiness, and runs the tests. `make ci` also runs the race detector. Do not claim a check passed unless it was run successfully. If a required check cannot run, report the exact blocker.

PostgreSQL-backed tests require `ATLAS_TEST_DATABASE_URL`. The test helper skips them when this variable is absent, so a successful `go test`, `make check`, or `make ci` process is not proof that database integration tests ran. Before required verification:

- Start the pinned PostgreSQL service and export the test environment, for example with `mise exec -- make db-up`, followed by `set -a; source .env; set +a`.
- Remember that merely having an `.env` file is insufficient: Docker Compose reads it, but Go tests only see variables exported into their process environment.
- Confirm `ATLAS_TEST_DATABASE_URL` is non-empty and the test database is reachable. If PostgreSQL, Docker, or the required environment is unavailable, report that exact blocker and do not claim database coverage.
- Treat skipped integration tests as not run. Inspect verbose or JSON test output when necessary to distinguish skips from passes.
- Keep database test expectations independent of the host timezone: normalize expected `timestamptz` values to UTC before structural comparison, compare instants when location identity is irrelevant, and explicitly assert UTC when it is part of the repository contract.
- Respect PostgreSQL timestamp precision in boundary fixtures. Use at least one microsecond outside an inclusive or exclusive boundary instead of nanoseconds that PostgreSQL may round onto the boundary.

Go may reuse cached test results even when an external database or extension has changed. Before handoff, run the complete suite without the test cache in addition to the standard targets:

```sh
mise exec -- go test -count=1 ./...
mise exec -- go test -race -count=1 ./...
```

When database schema, migrations, PostgreSQL versions, or extensions such as pgvector are involved, prefer `mise exec -- make db-reset` before the uncached suite so verification uses the pinned disposable environment. Do not describe cached output as fresh execution, and do not claim local CI parity unless the required integration tests actually ran against that environment.

When adding another language, generated artifact, or toolchain, extend `make fmt`, `make check`, and `make ci` so its formatter, linter or static analysis, dependency validation, and tests are enforced locally and in CI. Pin tool versions instead of relying on an ambient installation.

## Engineering expectations

- Prefer focused files and explicit composition over large modules with mixed responsibilities.
- Preserve existing architecture and naming unless correctness or safety requires a change.
- Validate inputs at system boundaries and keep authorization close to protected actions.
- Keep ingestion deterministic and retain original-source identity and traceability.
- Add the narrowest effective test whenever behavior changes.
- Make schema changes additive and rollout-safe; use UUID primary keys, audit columns, `timestamptz`, and deliberate indexes for business tables.
- Keep frontend routes and components thin, extract data access into domain hooks, and validate non-trivial UI work on desktop and mobile widths.
- Avoid unrelated refactors and comments that merely restate code.

Review every change for correctness, data integrity, security, maintainability, responsive behavior where relevant, migration safety, and adequate test coverage.
