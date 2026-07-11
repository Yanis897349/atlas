CREATE TYPE daily_brief_citation_kind AS ENUM ('source_record', 'upcoming_event');

CREATE TABLE daily_briefs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    region economic_region NOT NULL,
    publication_window_start timestamptz NOT NULL,
    publication_window_end timestamptz NOT NULL,
    event_window_start timestamptz NOT NULL,
    event_window_end timestamptz NOT NULL,
    provider text NOT NULL,
    model text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT chk_daily_briefs_publication_window CHECK (publication_window_end >= publication_window_start),
    CONSTRAINT chk_daily_briefs_event_window CHECK (event_window_end >= event_window_start),
    CONSTRAINT chk_daily_briefs_provider_nonempty CHECK (btrim(provider) <> ''),
    CONSTRAINT chk_daily_briefs_model_nonempty CHECK (btrim(model) <> ''),
    CONSTRAINT chk_daily_briefs_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_daily_briefs_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_daily_briefs_region_created_at_id
    ON daily_briefs (region, created_at DESC, id ASC);

CREATE TABLE daily_brief_sections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    daily_brief_id uuid NOT NULL REFERENCES daily_briefs (id) ON DELETE CASCADE,
    position integer NOT NULL,
    heading text NOT NULL,
    content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_daily_brief_sections_brief_position UNIQUE (daily_brief_id, position),
    CONSTRAINT chk_daily_brief_sections_position CHECK (position >= 0),
    CONSTRAINT chk_daily_brief_sections_heading_nonempty CHECK (btrim(heading) <> ''),
    CONSTRAINT chk_daily_brief_sections_content_nonempty CHECK (btrim(content) <> ''),
    CONSTRAINT chk_daily_brief_sections_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_daily_brief_sections_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_daily_brief_sections_daily_brief_id
    ON daily_brief_sections (daily_brief_id);

CREATE TABLE daily_brief_citations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    daily_brief_section_id uuid NOT NULL REFERENCES daily_brief_sections (id) ON DELETE CASCADE,
    position integer NOT NULL,
    citation_kind daily_brief_citation_kind NOT NULL,
    source_record_id uuid REFERENCES source_records (id),
    economic_event_id uuid REFERENCES economic_events (id),
    source text NOT NULL,
    source_url text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_daily_brief_citations_section_position UNIQUE (daily_brief_section_id, position),
    CONSTRAINT chk_daily_brief_citations_position CHECK (position >= 0),
    CONSTRAINT chk_daily_brief_citations_reference CHECK (
        (citation_kind = 'source_record' AND source_record_id IS NOT NULL AND economic_event_id IS NULL)
        OR
        (citation_kind = 'upcoming_event' AND source_record_id IS NULL AND economic_event_id IS NOT NULL)
    ),
    CONSTRAINT chk_daily_brief_citations_source_nonempty CHECK (btrim(source) <> ''),
    CONSTRAINT chk_daily_brief_citations_source_url_nonempty CHECK (btrim(source_url) <> ''),
    CONSTRAINT chk_daily_brief_citations_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_daily_brief_citations_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_daily_brief_citations_daily_brief_section_id
    ON daily_brief_citations (daily_brief_section_id);

CREATE INDEX ix_daily_brief_citations_source_record_id
    ON daily_brief_citations (source_record_id)
    WHERE source_record_id IS NOT NULL;

CREATE INDEX ix_daily_brief_citations_economic_event_id
    ON daily_brief_citations (economic_event_id)
    WHERE economic_event_id IS NOT NULL;
