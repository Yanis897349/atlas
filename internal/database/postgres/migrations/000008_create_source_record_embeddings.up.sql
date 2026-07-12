CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

CREATE TABLE source_record_embeddings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_record_id uuid NOT NULL REFERENCES source_records (id) ON DELETE CASCADE,
    provider text NOT NULL,
    model text NOT NULL,
    embedding public.vector NOT NULL,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    created_by text NOT NULL,
    updated_by text NOT NULL,
    CONSTRAINT uq_source_record_embeddings_record_provider_model UNIQUE (source_record_id, provider, model),
    CONSTRAINT chk_source_record_embeddings_provider_nonempty CHECK (btrim(provider) <> ''),
    CONSTRAINT chk_source_record_embeddings_model_nonempty CHECK (btrim(model) <> ''),
    CONSTRAINT chk_source_record_embeddings_embedding_cosine_norm
        CHECK ((embedding OPERATOR(public.<=>) embedding) = 0),
    CONSTRAINT chk_source_record_embeddings_created_by_nonempty CHECK (btrim(created_by) <> ''),
    CONSTRAINT chk_source_record_embeddings_updated_by_nonempty CHECK (btrim(updated_by) <> '')
);

CREATE INDEX ix_source_record_embeddings_provider_model
    ON source_record_embeddings (provider, model);
