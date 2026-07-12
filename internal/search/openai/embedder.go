package openai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	openaiapi "github.com/Yanis897349/atlas/internal/openai"
	"github.com/Yanis897349/atlas/internal/search"
)

const (
	maxBatchSize     = 2048
	maxRequestBytes  = 4 << 20
	maxResponseBytes = 128 << 20
)

// HTTPClient executes OpenAI Embeddings API requests.
type HTTPClient = openaiapi.HTTPClient

// Embedder produces source-record vectors through the OpenAI Embeddings API.
type Embedder struct {
	apiKey        string
	model         string
	client        HTTPClient
	endpoint      string
	requestBudget time.Duration
}

var _ search.Embedder = (*Embedder)(nil)

// Embed creates vectors for an ordered source-record input batch.
func (embedder *Embedder) Embed(
	ctx context.Context,
	inputs []search.EmbeddingInput,
) (search.EmbeddingBatch, error) {
	requestContext, cancel := context.WithTimeout(ctx, embedder.requestBudget)
	defer cancel()

	requestBody, err := newRequest(requestContext, embedder.model, inputs)
	if err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("encode OpenAI embeddings request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		embedder.endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("create OpenAI Embeddings API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+embedder.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := embedder.client.Do(request)
	if err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("send OpenAI Embeddings API request: %w", err)
	}
	if response == nil {
		return search.EmbeddingBatch{}, errors.New("send OpenAI Embeddings API request: HTTP client returned no response")
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("read OpenAI Embeddings API response: %w", err)
	}
	if len(responseBody) > maxResponseBytes {
		return search.EmbeddingBatch{}, fmt.Errorf(
			"read OpenAI Embeddings API response: body exceeds %d bytes",
			maxResponseBytes,
		)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return search.EmbeddingBatch{}, providerError(response.StatusCode, responseBody)
	}

	batch, err := decodeResponse(responseBody, embedder.model, inputs)
	if err != nil {
		return search.EmbeddingBatch{}, fmt.Errorf("decode OpenAI Embeddings API response: %w", err)
	}
	return batch, nil
}
