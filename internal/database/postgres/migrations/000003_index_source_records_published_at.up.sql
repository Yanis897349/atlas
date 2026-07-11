CREATE INDEX ix_source_records_published_at_id
    ON source_records (published_at DESC, id ASC);
