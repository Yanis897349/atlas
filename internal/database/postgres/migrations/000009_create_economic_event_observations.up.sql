CREATE TABLE economic_event_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    economic_event_id uuid NOT NULL REFERENCES economic_events (id) ON DELETE CASCADE,
    source text NOT NULL,
    source_observation_id text NOT NULL,
    source_url text NOT NULL,
    observed_at timestamptz NOT NULL,
    consensus_value text,
    previous_value text,
    actual_value text,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_economic_event_observations_event_source_identity
        UNIQUE (economic_event_id, source, source_observation_id),
    CONSTRAINT chk_economic_event_observations_source_nonempty CHECK (source !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_source_identity_nonempty
        CHECK (source_observation_id !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_source_url_nonempty CHECK (source_url !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_consensus_nonempty
        CHECK (consensus_value IS NULL OR consensus_value !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_previous_nonempty
        CHECK (previous_value IS NULL OR previous_value !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_actual_nonempty
        CHECK (actual_value IS NULL OR actual_value !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_value_required
        CHECK (consensus_value IS NOT NULL OR previous_value IS NOT NULL OR actual_value IS NOT NULL),
    CONSTRAINT chk_economic_event_observations_created_by_nonempty CHECK (created_by !~ '^[[:space:]]*$'),
    CONSTRAINT chk_economic_event_observations_updated_by_nonempty CHECK (updated_by !~ '^[[:space:]]*$')
);

CREATE INDEX ix_economic_event_observations_event_observed_at_id
    ON economic_event_observations (economic_event_id, observed_at DESC, id ASC);
