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
		return nil, fmt.Errorf("OpenAI embeddings request exceeds %d bytes", maxRequestBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return request, nil
}
