package search

import "context"

type embedderStub struct {
	batch  EmbeddingBatch
	err    error
	calls  int
	inputs []EmbeddingInput
}

func (embedder *embedderStub) Embed(_ context.Context, inputs []EmbeddingInput) (EmbeddingBatch, error) {
	embedder.calls++
	embedder.inputs = append([]EmbeddingInput(nil), inputs...)
	return embedder.batch, embedder.err
}

type panicEmbedder struct{}

func (panicEmbedder) Embed(context.Context, []EmbeddingInput) (EmbeddingBatch, error) {
	panic("embedding provider must not run")
}
