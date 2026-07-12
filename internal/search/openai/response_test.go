package openai

import (
	"strings"
	"testing"

	"github.com/Yanis897349/atlas/internal/search"
)

func TestDecodeResponseRejectsMalformedResponses(t *testing.T) {
	inputs := []search.EmbeddingInput{
		{SourceRecordID: "record-1", Text: "Headline one"},
		{SourceRecordID: "record-2", Text: "Headline two"},
	}
	tests := []struct {
		name     string
		body     string
		contains string
	}{
		{name: "invalid JSON", body: `{`, contains: "decode response envelope"},
		{name: "unexpected envelope object", body: `{
			"object":"embedding","model":"model","data":[]
		}`, contains: `unexpected response object "embedding"`},
		{name: "missing model", body: `{
			"object":"list","data":[]
		}`, contains: `response model "" does not match requested model "model"`},
		{name: "different model", body: `{
			"object":"list","model":"other-model","data":[]
		}`, contains: `response model "other-model" does not match requested model "model"`},
		{name: "missing result", body: `{
			"object":"list","model":"model",
			"data":[{"object":"embedding","embedding":[0.1],"index":0}]
		}`, contains: "contains 1 embeddings for 2 inputs"},
		{name: "extra result", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"embedding","embedding":[0.1],"index":0},
				{"object":"embedding","embedding":[0.2],"index":1},
				{"object":"embedding","embedding":[0.3],"index":2}
			]
		}`, contains: "contains 3 embeddings for 2 inputs"},
		{name: "unexpected result object", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"vector","embedding":[0.1],"index":0},
				{"object":"embedding","embedding":[0.2],"index":1}
			]
		}`, contains: `result 0 has unexpected object "vector"`},
		{name: "missing index", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"embedding","embedding":[0.1]},
				{"object":"embedding","embedding":[0.2],"index":1}
			]
		}`, contains: "result 0 index is required"},
		{name: "negative index", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"embedding","embedding":[0.1],"index":-1},
				{"object":"embedding","embedding":[0.2],"index":1}
			]
		}`, contains: "index -1 is outside input range"},
		{name: "out of range index", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"embedding","embedding":[0.1],"index":0},
				{"object":"embedding","embedding":[0.2],"index":2}
			]
		}`, contains: "index 2 is outside input range"},
		{name: "duplicate index", body: `{
			"object":"list","model":"model",
			"data":[
				{"object":"embedding","embedding":[0.1],"index":0},
				{"object":"embedding","embedding":[0.2],"index":0}
			]
		}`, contains: "result index 0 is duplicated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeResponse([]byte(test.body), "model", inputs)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("decodeResponse() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestDecodeResponseLeavesVectorValidationToSearchBoundary(t *testing.T) {
	inputs := []search.EmbeddingInput{{SourceRecordID: "record-1", Text: "Headline"}}
	batch, err := decodeResponse([]byte(`{
		"object":"list","model":"model",
		"data":[{"object":"embedding","embedding":[],"index":0}]
	}`), "model", inputs)
	if err != nil {
		t.Fatalf("decodeResponse() error = %v", err)
	}
	if len(batch.Embeddings) != 1 || len(batch.Embeddings[0].Vector) != 0 {
		t.Fatalf("decodeResponse() = %#v, want empty vector passed to domain validation", batch)
	}
}
