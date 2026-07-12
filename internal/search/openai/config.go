package openai

import openaiapi "github.com/Yanis897349/atlas/internal/openai"

const (
	defaultEndpoint = "https://api.openai.com/v1/embeddings"
)

// Config configures an OpenAI source-record embedder.
type Config = openaiapi.Config

// NewEmbedder returns a validated OpenAI source-record embedder.
func NewEmbedder(config Config) (*Embedder, error) {
	resolved, err := openaiapi.ResolveConfig(config, defaultEndpoint)
	if err != nil {
		return nil, err
	}

	return &Embedder{
		apiKey:        resolved.APIKey,
		model:         resolved.Model,
		client:        resolved.Client,
		endpoint:      resolved.Endpoint,
		requestBudget: resolved.RequestBudget,
	}, nil
}
