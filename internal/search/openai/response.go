package openai

import (
	"encoding/json"
	"fmt"

	openaiapi "github.com/Yanis897349/atlas/internal/openai"
	"github.com/Yanis897349/atlas/internal/search"
)

type embeddingsResponse struct {
	Object string                  `json:"object"`
	Data   []embeddingResponseItem `json:"data"`
	Model  string                  `json:"model"`
}

type embeddingResponseItem struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     *int      `json:"index"`
}

func decodeResponse(
	body []byte,
	model string,
	inputs []search.EmbeddingInput,
) (search.EmbeddingBatch, error) {
	var response embeddingsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("decode response envelope: %w", err)
	}
	if response.Object != "list" {
		return search.EmbeddingBatch{}, fmt.Errorf("unexpected response object %q", response.Object)
	}
	if response.Model != model {
		return search.EmbeddingBatch{}, fmt.Errorf(
			"response model %q does not match requested model %q",
			response.Model,
			model,
		)
	}
	if len(response.Data) != len(inputs) {
		return search.EmbeddingBatch{}, fmt.Errorf(
			"response contains %d embeddings for %d inputs",
			len(response.Data),
			len(inputs),
		)
	}

	embeddings := make([]search.ProviderEmbedding, len(inputs))
	seen := make([]bool, len(inputs))
	for resultPosition, result := range response.Data {
		if result.Object != "embedding" {
			return search.EmbeddingBatch{}, fmt.Errorf(
				"result %d has unexpected object %q",
				resultPosition,
				result.Object,
			)
		}
		if result.Index == nil {
			return search.EmbeddingBatch{}, fmt.Errorf("result %d index is required", resultPosition)
		}
		index := *result.Index
		if index < 0 || index >= len(inputs) {
			return search.EmbeddingBatch{}, fmt.Errorf(
				"result %d index %d is outside input range [0, %d)",
				resultPosition,
				index,
				len(inputs),
			)
		}
		if seen[index] {
			return search.EmbeddingBatch{}, fmt.Errorf("result index %d is duplicated", index)
		}
		seen[index] = true
		embeddings[index] = search.ProviderEmbedding{
			SourceRecordID: inputs[index].SourceRecordID,
			Vector:         result.Embedding,
		}
	}
	for index, present := range seen {
		if !present {
			return search.EmbeddingBatch{}, fmt.Errorf("response is missing result index %d", index)
		}
	}

	return search.EmbeddingBatch{
		Provider:   "openai",
		Model:      response.Model,
		Embeddings: embeddings,
	}, nil
}

func providerError(statusCode int, body []byte) error {
	return openaiapi.ProviderError("OpenAI Embeddings API", statusCode, body)
}
