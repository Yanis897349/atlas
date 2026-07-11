CREATE TABLE watchlist_event_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    watchlist_instrument_id uuid NOT NULL REFERENCES watchlist_instruments (id) ON DELETE CASCADE,
    economic_event_id uuid NOT NULL REFERENCES economic_events (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_watchlist_event_links_instrument_event UNIQUE (watchlist_instrument_id, economic_event_id),
    CONSTRAINT chk_watchlist_event_links_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_watchlist_event_links_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_watchlist_event_links_economic_event_id
    ON watchlist_event_links (economic_event_id);
