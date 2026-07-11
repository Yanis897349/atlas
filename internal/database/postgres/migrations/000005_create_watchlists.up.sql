CREATE TABLE watchlists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT chk_watchlists_name_nonempty CHECK (btrim(name) <> ''),
    CONSTRAINT chk_watchlists_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_watchlists_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_watchlists_created_at_id
    ON watchlists (created_at DESC, id ASC);

CREATE TABLE watchlist_instruments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    watchlist_id uuid NOT NULL REFERENCES watchlists (id) ON DELETE CASCADE,
    position integer NOT NULL,
    symbol text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_watchlist_instruments_watchlist_position UNIQUE (watchlist_id, position),
    CONSTRAINT uq_watchlist_instruments_watchlist_symbol UNIQUE (watchlist_id, symbol),
    CONSTRAINT chk_watchlist_instruments_position CHECK (position >= 0),
    CONSTRAINT chk_watchlist_instruments_symbol_nonempty CHECK (btrim(symbol) <> ''),
    CONSTRAINT chk_watchlist_instruments_symbol_canonical CHECK (symbol = btrim(symbol) AND symbol = upper(symbol)),
    CONSTRAINT chk_watchlist_instruments_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_watchlist_instruments_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_watchlist_instruments_watchlist_id
    ON watchlist_instruments (watchlist_id);
