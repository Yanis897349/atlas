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

PostgreSQL integration tests run when `ATLAS_TEST_DATABASE_URL` is set. The test account must be allowed to create isolated schemas and install the pgvector extension; CI provisions a pgvector-enabled PostgreSQL service and runs these tests automatically.

### Run PostgreSQL locally

Docker Compose provides PostgreSQL 17 with pgvector enabled and separate `atlas` application and `atlas_test` integration-test databases. Start it, copy the development configuration once, and export that configuration before running commands so the PostgreSQL-backed tests are not skipped:

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

Atlas application commands require `ATLAS_DATABASE_URL`. Keep this application database separate from `ATLAS_TEST_DATABASE_URL`, which integration tests isolate and clean up. RSS ingestion also requires `ATLAS_OPENAI_API_KEY` and `ATLAS_OPENAI_EMBEDDING_MODEL` because it automatically indexes the canonical records it stores. Apply migrations explicitly before ingestion:

```sh
export ATLAS_DATABASE_URL='postgres://postgres:postgres@localhost:5432/atlas?sslmode=disable'
export ATLAS_OPENAI_API_KEY='replace-with-an-api-key'
export ATLAS_OPENAI_EMBEDDING_MODEL='replace-with-an-embeddings-api-model'
mise exec -- go run ./cmd/atlas migrate
mise exec -- go run ./cmd/atlas ingest-rss
mise exec -- go run ./cmd/atlas ingest-bls
mise exec -- go run ./cmd/atlas ingest-fed
mise exec -- go run ./cmd/atlas ingest-ecb
mise exec -- go run ./cmd/atlas ingest-bea
mise exec -- go run ./cmd/atlas ingest-census
mise exec -- go run ./cmd/atlas ingest-eurostat
mise exec -- go run ./cmd/atlas ingest-spglobal
mise exec -- go run ./cmd/atlas upcoming-events \
  --region united_states \
  --from 2026-07-01T00:00:00Z \
  --to 2026-08-01T00:00:00Z \
  --limit 25
mise exec -- go run ./cmd/atlas daily-brief-input \
  --region united_states \
  --publication-from 2026-07-10T12:00:00Z \
  --publication-to 2026-07-11T12:00:00Z \
  --source-record-limit 50 \
  --event-from 2026-07-11T12:00:00Z \
  --event-to 2026-07-18T12:00:00Z \
  --upcoming-event-limit 25
export ATLAS_OPENAI_MODEL='replace-with-a-responses-api-model'
mise exec -- go run ./cmd/atlas daily-brief \
  --region united_states \
  --publication-from 2026-07-10T12:00:00Z \
  --publication-to 2026-07-11T12:00:00Z \
  --source-record-limit 50 \
  --event-from 2026-07-11T12:00:00Z \
  --event-to 2026-07-18T12:00:00Z \
  --upcoming-event-limit 25
mise exec -- go run ./cmd/atlas daily-briefs \
  --region united_states \
  --from 2026-07-01T00:00:00Z \
  --to 2026-08-01T00:00:00Z \
  --limit 25
mise exec -- go run ./cmd/atlas index-source-records \
  --from 2026-07-10T12:00:00Z \
  --to 2026-07-11T12:00:00Z \
  --limit 50 \
  --actor search-indexer
mise exec -- go run ./cmd/atlas search-source-records \
  --query 'central bank policy outlook' \
  --source investinglive \
  --from 2026-07-10T12:00:00Z \
  --to 2026-07-11T12:00:00Z \
  --limit 10
mise exec -- go run ./cmd/atlas economic-event-context \
  --event-id 00000000-0000-0000-0000-000000000001 \
  --from 2026-07-10T12:00:00Z \
  --to 2026-07-11T12:00:00Z \
  --limit 10
mise exec -- go run ./cmd/atlas create-watchlist \
  --name 'Macro focus' \
  --actor analyst \
  --symbol EURUSD \
  --symbol SPY
mise exec -- go run ./cmd/atlas update-watchlist \
  --id 00000000-0000-0000-0000-000000000001 \
  --name 'Updated macro focus' \
  --actor editor \
  --symbol DXY \
  --symbol BRK.B
mise exec -- go run ./cmd/atlas delete-watchlist \
  --id 00000000-0000-0000-0000-000000000001
mise exec -- go run ./cmd/atlas watchlist --id 00000000-0000-0000-0000-000000000001
mise exec -- go run ./cmd/atlas watchlists --limit 25
mise exec -- go run ./cmd/atlas link-watchlist-event \
  --id 00000000-0000-0000-0000-000000000001 \
  --symbol EURUSD \
  --event-id 00000000-0000-0000-0000-000000000002 \
  --actor analyst
mise exec -- go run ./cmd/atlas link-watchlist-events \
  --id 00000000-0000-0000-0000-000000000001 \
  --from 2026-07-01T00:00:00Z \
  --to 2026-08-01T00:00:00Z \
  --limit 25 \
  --actor classifier
mise exec -- go run ./cmd/atlas unlink-watchlist-event \
  --id 00000000-0000-0000-0000-000000000001 \
  --symbol EURUSD \
  --event-id 00000000-0000-0000-0000-000000000002
mise exec -- go run ./cmd/atlas watchlist-events \
  --id 00000000-0000-0000-0000-000000000001 \
  --symbol EURUSD \
  --limit 25
```

`migrate` applies pending schema changes transactionally and is safe to repeat. `ingest-rss` performs one bounded InvestingLive fetch-to-persist cycle, then embeds the exact canonical stored titles in feed order and atomically persists their provider- and model-specific pgvector representations under the RSS ingestion audit identity. Empty feeds make no embedding request, repeated cycles safely retry both source and embedding upserts, and an indexing failure leaves the canonical source rows persisted while returning an error without ingestion success output. `ingest-bls`, `ingest-fed`, `ingest-ecb`, `ingest-bea`, `ingest-census`, `ingest-eurostat`, and `ingest-spglobal` ingest supported official calendar releases without source-record embedding. All ingestion commands exit after one cycle; scheduling, continuous workers, recovery queues, and non-RSS automatic indexing are intentionally not part of these commands.

`upcoming-events` reads one supported region (`united_states` or `eurozone`) over an inclusive RFC 3339 time window. Its limit must be from 1 through 100. The command emits a JSON array ordered by scheduled time and event ID, retaining each event's source identity and citation URL; it does not ingest or modify records.

`daily-brief-input` reads recent source records and region-specific upcoming events over separate inclusive RFC 3339 windows. The source-record and upcoming-event limits are independent and must each be from 1 through 100. The command emits a deterministic JSON envelope containing the UTC query windows, newest-first source records, and chronologically ordered events with their source identities and citation URLs; it does not generate prose, call an AI provider, or modify records.

`daily-brief` accepts the same windows and limits, requires `ATLAS_OPENAI_API_KEY` and `ATLAS_OPENAI_MODEL`, and sends the assembled deterministic input to the OpenAI Responses API. After validating the generated sections and resolving canonical source-record or upcoming-event citations from PostgreSQL, it atomically persists an immutable brief with its UUID, input windows, provider and model provenance, ordered content, citations, and audit metadata. The command emits that complete stored record as JSON; provider-supplied URLs are never trusted, and failed generation or validation does not create a brief. Each invocation performs one bounded provider request without retries. Regeneration policy, scheduling, HTTP delivery, and UI presentation are not part of this command.

`daily-briefs` reads persisted briefs for one supported region over an inclusive RFC 3339 creation window, with a limit from 1 through 100. It emits a JSON array ordered by creation time newest first and UUID for ties, preserving each brief's original input windows, provider and model provenance, ordered sections and canonical citations, and audit metadata. The command does not call an AI provider or modify stored briefs.

`index-source-records` remains available for bounded backfills. It reads up to 100 canonical source records from one inclusive RFC 3339 publication window, newest first with UUID tie-breaking, embeds each exact persisted title through the OpenAI Embeddings API, and atomically inserts or replaces its provider- and model-specific pgvector representation with the supplied audit actor. The command emits deterministic JSON metadata in retrieval order containing each source-record UUID, normalized provider and model provenance, and vector dimension without exposing vectors; an empty window emits `[]` without calling the provider. Each invocation performs the bounded provider requests required by input count and encoded payload size without retries, and repeated indexing is idempotent for unchanged vectors. Scheduling, pagination, non-RSS hooks, and body or content embeddings remain deferred.

`search-source-records` accepts one exact non-blank text query, an optional single source filter, optional paired inclusive RFC 3339 publication bounds, and a result limit from 1 through 100. Source filters are trimmed and matched by case-sensitive equality against the canonical source; omitting the source filter searches every source. Publication bounds filter canonical publication timestamps and must be supplied together as `--from` and `--to`; omitting both leaves publication time unbounded. The command embeds the query through the configured OpenAI Embeddings API model and retrieves only stored vectors with matching normalized provider, model, and dimension, ordered by exact pgvector cosine distance and source-record UUID for ties. It emits each complete canonical source record with UTC timestamps, persistence audit metadata, provider and model provenance, and cosine distance; no matches are represented as `[]`. One-sided publication windows, multiple-source expressions, pagination, hybrid or lexical ranking, HTTP delivery, and UI presentation remain deferred.

`economic-event-context` accepts one canonical economic-event UUID, a required inclusive RFC 3339 source-publication window, and a source-record limit from 1 through 100. It loads the complete source-cited event, embeds its exact persisted name through the configured OpenAI Embeddings API model, and retrieves only source records within the publication window whose stored vectors match the normalized provider, model, and dimension. The command emits a deterministic JSON envelope containing the complete event, normalized UTC window, and complete canonical source records in exact cosine-distance then UUID order with persistence audit metadata and model provenance; no matches are represented as `[]`. A missing event returns a not-found error without calling the provider or emitting JSON, and validation, configuration, provider, database, or cancellation failures emit no partial result. Provider-neutral context assembly also retrieves a bounded repository-ordered set of latest source observation snapshots with exact consensus, previous, and actual values, source citations, and UTC observation and audit metadata; observation ingestion, revision history, surprise calculations, generated analysis, command output, HTTP delivery, and UI presentation remain deferred.

`create-watchlist` atomically persists one user-authored watchlist. It requires a name, an audit actor, and one or more ordered `--symbol` flags; names and actors are trimmed, while symbols are trimmed, canonicalized to uppercase, and rejected when empty or duplicated after normalization. The command emits the complete stored definition with its UUID and audit metadata as JSON.

`update-watchlist` atomically replaces one persisted watchlist definition by UUID. It applies the same name, actor, and ordered-symbol validation and normalization as creation, preserves the original creation metadata, updates the modification audit metadata, and emits the complete updated definition as JSON. A valid UUID that does not exist returns a not-found error without emitting JSON; partial definition updates are not supported.

`delete-watchlist` atomically deletes one persisted watchlist definition by UUID, including its instruments. Invalid UUIDs are rejected before database setup, successful deletion emits no output, and a valid UUID that does not exist returns a not-found error without output.

`watchlist` reads one persisted watchlist definition by UUID and emits its complete definition, ordered symbols, and audit metadata as JSON. Invalid UUIDs are rejected before database setup, and a valid UUID that does not exist returns a not-found error without emitting JSON; the command never modifies the stored definition.

`watchlists` reads up to 100 persisted watchlist definitions. It emits a JSON array ordered by creation time newest first and UUID for ties, preserving each definition's ordered symbols and audit metadata; an empty result is emitted as `[]`.

`link-watchlist-event` atomically associates one watchlist instrument with one canonical economic event. It requires the watchlist and event UUIDs, an instrument symbol belonging to that watchlist, and an explicit audit actor; actors are trimmed and symbols are trimmed and canonicalized to uppercase. Missing references return a not-found error, duplicate associations return a uniqueness error, and failures emit no JSON. A successful command emits the complete immutable link, link audit metadata, and nested source-cited economic event with its persistence metadata.

`link-watchlist-events` retrieves one globally bounded inclusive RFC 3339 window of supported United States and Eurozone economic-event candidates, applies the deterministic relevance rules to every symbol in one persisted watchlist, and atomically creates or loads the relevant links. It requires a watchlist UUID and explicit audit actor, emits complete source-cited links in stable symbol-then-event order, represents no links as `[]`, and is idempotent across repeated invocations.

`unlink-watchlist-event` atomically removes one exact association identified by its watchlist UUID, canonicalized instrument symbol, and economic-event UUID. Successful deletion emits no output, missing references or associations return a not-found error without output, and the watchlist, instrument, economic event, and unrelated links are preserved.

`watchlist-events` reads up to 100 linked economic events for one watchlist instrument. It canonicalizes the supplied symbol and emits complete links as a JSON array ordered by event time and event UUID, with an empty result represented as `[]`. Bulk unlinking, automated relevance inference, market data, scheduling, HTTP delivery, and UI presentation remain deferred.

The provider-neutral watchlist domain classifies all supported United States economic-event types as relevant to `SPY`, `DXY`, and `EURUSD`, and all supported Eurozone event types as relevant only to `EURUSD`. Its application service retrieves one bounded inclusive window of canonical event candidates, loads one persisted watchlist, applies those rules across its ordered symbols and candidate order, and atomically persists relevant results in stable symbol-then-event order. Repeated and duplicate associations are successful no-ops, returned links contain the canonical source-cited events resolved from PostgreSQL, and unsupported instruments, regions, and event types fail explicitly. AI inference, market data, scheduling, HTTP delivery, and UI presentation remain deferred.
