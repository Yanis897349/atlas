package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Yanis897349/atlas/internal/search"
)

type embeddingsRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

var errRequestExceedsLimit = errors.New("OpenAI embeddings request exceeds byte limit")

func newRequest(ctx context.Context, model string, inputs []search.EmbeddingInput) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, errors.New("embedding input batch is required")
	}
	if len(inputs) > maxBatchSize {
		return nil, fmt.Errorf("embedding input batch must not exceed %d items", maxBatchSize)
	}

	texts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		texts = append(texts, input.Text)
	}
	request, err := json.Marshal(embeddingsRequest{
		Model:          model,
		Input:          texts,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, err
	}
	if len(request) > maxRequestBytes {
		return nil, fmt.Errorf("OpenAI embeddings request exceeds %d bytes: %w", maxRequestBytes, errRequestExceedsLimit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return request, nil
}

func nextRequest(
	ctx context.Context,
	model string,
	inputs []search.EmbeddingInput,
) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if len(inputs) == 0 {
		return nil, 0, errors.New("embedding input batch is required")
	}

	high := min(len(inputs), maxBatchSize)
	low := 1
	var request []byte
	count := 0
	for low <= high {
		candidateCount := low + (high-low)/2
		candidate, err := newRequest(ctx, model, inputs[:candidateCount])
		if err == nil {
			request = candidate
			count = candidateCount
			low = candidateCount + 1
			continue
		}
		if errors.Is(err, errRequestExceedsLimit) {
			high = candidateCount - 1
			continue
		}
		return nil, 0, err
	}
	if count == 0 {
		_, err := newRequest(ctx, model, inputs[:1])
		return nil, 0, err
	}
	return request, count, nil
}
