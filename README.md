# Atlas

Atlas is an open-source macro intelligence platform that turns economic news, scheduled events, and market data into cited daily context, evolving narratives, and event-driven watchlists.

It is designed to help users understand **why markets are moving**, which events matter next, and how macro narratives change over time. Atlas is a decision-support system, not a prediction engine.

> [!NOTE]
> Atlas is currently in the early design and development stage. The capabilities and architecture below describe the intended product direction, not a production-ready release.

## Why Atlas?

Financial information is fragmented across news articles, economic calendars, government publications, earnings reports, and market data. Atlas brings these sources into a common model and uses them to answer questions such as:

- What are the dominant macro narratives?
- Which events should I monitor next?
- Why is this asset moving?
- How has this narrative evolved over time?
- Which upcoming events could affect my watchlist?

## Core principles

- **Source-first:** every generated statement should be traceable to its original source.
- **Structured over unstructured:** raw articles become structured events, entities, and narratives.
- **Context over summaries:** Atlas explains why information matters instead of only summarizing it.
- **Decision support, not prediction:** outputs inform analysis without claiming certainty about future markets.
- **Deterministic foundations:** the data pipeline remains deterministic, with AI used for enrichment, classification, and synthesis.

## Inputs and outputs

Atlas is intended to ingest:

- Financial news and RSS feeds
- Economic and central bank calendars
- Government publications
- Earnings calendars
- Market data
- User-defined watchlists

From these sources, Atlas will produce:

- A daily macro brief
- Event-specific intelligence
- Personalized market watchlists
- Narrative tracking and historical context
- Asset-impact analysis
- Citation-backed explanations
- Change detection between reports

## Initial scope

The first release focuses on the United States and Eurozone, particularly the Federal Reserve and European Central Bank. Initial topic coverage includes:

- Inflation and employment
- Interest rates
- GDP, PMI, and retail sales
- Foreign exchange
- Major equity indices

## Core capabilities

### Daily brief

The daily workflow will:

1. Ingest new information.
2. Normalize sources and remove duplicates.
3. Extract entities and identify economic events.
4. Group related stories into narratives.
5. Produce a citation-backed macro brief.
6. Generate personalized watchlists.

### Event intelligence

Important macro events will combine consensus, previous and actual values, historical data, related news, relevant assets, pre-event scenarios, and post-event analysis.

Initial event coverage is planned for CPI, GDP, nonfarm payrolls, central bank decisions, PMI, retail sales, and major earnings.

### Narrative tracking

Atlas will track long-running narratives such as “higher for longer,” a soft landing, European stagnation, oil supply disruption, or yen intervention risk.

Each narrative will include supporting and contradicting evidence, a confidence score, its first appearance and latest updates, related assets, and source citations.

## Architecture

The planned high-level data flow is:

```text
News APIs / RSS / Calendars / Market Data
                  │
                  ▼
          Ingestion Services
                  │
                  ▼
        Queue & Worker Pipeline
                  │
      ┌───────────┴───────────┐
      ▼                       ▼
Normalization            Enrichment
Deduplication            Entity Extraction
Source Scoring           Topic Classification
                         Event Linking
      └───────────┬───────────┘
                  ▼
      PostgreSQL + Object Storage
                  │
      ┌───────────┴───────────┐
      ▼                       ▼
 Retrieval Layer       Narrative Engine
      └───────────┬───────────┘
                  ▼
         Brief Generation Engine
                  │
                  ▼
          Next.js Dashboard
```

## Roadmap

### V1 — Foundations

- Source ingestion
- Economic calendar
- Daily macro brief
- Personalized watchlists
- Semantic search

### V2 — Context

- Event intelligence
- Asset-impact analysis
- Narrative tracking
- Historical comparisons

### V3 — Platform

- Advanced filtering
- Team dashboards
- Custom alerting
- API access

## Development

Atlas uses Go 1.26.5, pinned in `.mise.toml` and `go.mod`. Install and activate the pinned tools with mise:

```sh
mise install
```

During development, run `mise exec -- make fmt` to format Go files and `mise exec -- make check` after every change. The check target verifies formatting, runs the pinned golangci-lint suite, checks that the module files are tidy, and runs the tests. The CI target adds the race detector. GitHub Actions enforces the same checks on pushes and pull requests.

The initial supported source is the [InvestingLive RSS feed](https://investinglive.com/feed/). The RSS 2.0 adapter normalizes source items, and the PostgreSQL ingestion repository persists them using the source name and stable source-item identifier as an idempotency key. A later retrieval may correct stored metadata, while retries and older retrievals leave the existing row unchanged. Source item identity is a SHA-256 digest of the configured source and the entry GUID, falling back to the original URL when no GUID is present; exact repeated identities within one response are emitted once.

PostgreSQL integration tests run when `ATLAS_TEST_DATABASE_URL` is set. The test account must be allowed to create isolated schemas; CI provisions a PostgreSQL service and runs these tests automatically.

### Run PostgreSQL locally

Docker Compose provides PostgreSQL 17 with separate `atlas` application and `atlas_test` integration-test databases. Start it, copy the development configuration once, and export that configuration before running commands so the PostgreSQL-backed tests are not skipped:

```sh
cp -n .env.example .env
mise exec -- make db-up
set -a
. ./.env
set +a
mise exec -- make ci
```

`db-down` stops PostgreSQL while preserving its local volume. `db-reset` permanently deletes that volume, recreates both disposable development databases, and starts PostgreSQL again. Neither the Go application nor Make loads `.env` automatically, so export it in each new shell before running application commands or integration tests.

### Run one ingestion cycle

Atlas application commands require `ATLAS_DATABASE_URL`. Keep this application database separate from `ATLAS_TEST_DATABASE_URL`, which integration tests isolate and clean up. Apply migrations explicitly before ingestion:

```sh
export ATLAS_DATABASE_URL='postgres://postgres:postgres@localhost:5432/atlas?sslmode=disable'
mise exec -- go run ./cmd/atlas migrate
mise exec -- go run ./cmd/atlas ingest-rss
mise exec -- go run ./cmd/atlas ingest-bls
mise exec -- go run ./cmd/atlas ingest-fed
mise exec -- go run ./cmd/atlas ingest-ecb
mise exec -- go run ./cmd/atlas ingest-bea
mise exec -- go run ./cmd/atlas ingest-census
mise exec -- go run ./cmd/atlas ingest-eurostat
mise exec -- go run ./cmd/atlas upcoming-events \
  --region united_states \
  --from 2026-07-01T00:00:00Z \
  --to 2026-08-01T00:00:00Z \
  --limit 25
```

`migrate` applies pending schema changes transactionally and is safe to repeat. `ingest-rss` performs one bounded InvestingLive fetch-to-persist cycle, while `ingest-bls`, `ingest-fed`, `ingest-ecb`, `ingest-bea`, `ingest-census`, and `ingest-eurostat` do the same for supported releases from the official BLS calendar, regular meetings from the official Federal Reserve FOMC calendar, monetary policy meetings from the official ECB calendar, national GDP estimates from the official BEA release schedule, retail-sales releases from the official Census calendar, and current-year Euro-area quarterly GDP and monthly retail-sales releases from the official Eurostat calendar. All ingestion commands exit after one cycle and are idempotent: repeated cycles update newer retrieval metadata without creating duplicate records. Scheduling and continuous workers are intentionally not part of these commands.

`upcoming-events` reads one supported region (`united_states` or `eurozone`) over an inclusive RFC 3339 time window. Its limit must be from 1 through 100. The command emits a JSON array ordered by scheduled time and event ID, retaining each event's source identity and citation URL; it does not ingest or modify records.
