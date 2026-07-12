ALTER TABLE source_record_embeddings
    DROP CONSTRAINT chk_source_record_embeddings_embedding_nonzero,
    ADD CONSTRAINT chk_source_record_embeddings_embedding_cosine_norm
    CHECK ((embedding OPERATOR(public.<=>) embedding) = 0);
