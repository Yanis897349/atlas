ALTER TABLE source_record_embeddings
    ADD CONSTRAINT chk_source_record_embeddings_embedding_nonzero
    CHECK (public.vector_norm(embedding) > 0);
