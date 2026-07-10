CREATE TYPE economic_region AS ENUM ('united_states', 'eurozone');
CREATE TYPE economic_event_type AS ENUM (
    'inflation',
    'employment',
    'interest_rate_decision',
    'gdp',
    'pmi',
    'retail_sales'
);

CREATE TABLE economic_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    external_event_id text NOT NULL,
    name text NOT NULL,
    region economic_region NOT NULL,
    event_type economic_event_type NOT NULL,
    scheduled_at timestamptz NOT NULL,
    source_url text NOT NULL,
    retrieved_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_economic_events_source_external_event_id UNIQUE (source, external_event_id),
    CONSTRAINT chk_economic_events_source_nonempty CHECK (btrim(source) <> ''),
    CONSTRAINT chk_economic_events_external_event_id_nonempty CHECK (btrim(external_event_id) <> ''),
    CONSTRAINT chk_economic_events_name_nonempty CHECK (btrim(name) <> ''),
    CONSTRAINT chk_economic_events_source_url_nonempty CHECK (btrim(source_url) <> ''),
    CONSTRAINT chk_economic_events_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_economic_events_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_economic_events_region_scheduled_at
    ON economic_events (region, scheduled_at);
