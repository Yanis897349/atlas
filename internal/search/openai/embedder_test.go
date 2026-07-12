package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
)

func TestEmbedderRequestsOrderedFloatEmbeddingsAndRestoresIdentity(t *testing.T) {
	inputs := []search.EmbeddingInput{
		{SourceRecordID: "record-second", Text: "  Title kept exactly  "},
		{SourceRecordID: "record-first", Text: "Another headline"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/embeddings" {
			t.Errorf("request = %s %s, want POST /v1/embeddings", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("Authorization = %q, want bearer test credential", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var providerRequest embeddingsRequest
		if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
			t.Errorf("decode request: %v", err)
		}
		wantRequest := embeddingsRequest{
			Model:          "test-model",
			Input:          []string{"  Title kept exactly  ", "Another headline"},
			EncodingFormat: "float",
		}
		if !reflect.DeepEqual(providerRequest, wantRequest) {
			t.Errorf("provider request = %#v, want %#v", providerRequest, wantRequest)
		}

		writeResponse(t, response, http.StatusOK, `{
			"object":"list",
			"model":"test-model",
			"data":[
				{"object":"embedding","embedding":[0.4,0.5],"index":1},
				{"object":"embedding","embedding":[0.1,0.2],"index":0}
			]
		}`)
	}))
	defer server.Close()

	got, err := newTestEmbedder(t, server).Embed(t.Context(), inputs)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	want := search.EmbeddingBatch{
		Provider: "openai",
		Model:    "test-model",
		Embeddings: []search.ProviderEmbedding{
			{SourceRecordID: "record-second", Vector: []float32{0.1, 0.2}},
			{SourceRecordID: "record-first", Vector: []float32{0.4, 0.5}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Embed() = %#v, want %#v", got, want)
	}
}

func TestNewEmbedderUsesEmbeddingEndpointDefault(t *testing.T) {
	if embedder, err := NewEmbedder(Config{Model: "model"}); err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("NewEmbedder() = (%#v, %v), want shared configuration validation", embedder, err)
	}

	embedder, err := NewEmbedder(Config{APIKey: "key", Model: "model"})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	if embedder.endpoint != defaultEndpoint {
		t.Errorf("embedder defaults = %#v", embedder)
	}
}

func TestEmbedderRejectsBoundedRequestsBeforeHTTP(t *testing.T) {
	cancelledContext := func() context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	tests := []struct {
		name     string
		ctx      func() context.Context
		inputs   func() []search.EmbeddingInput
		contains string
	}{
		{name: "cancelled context", ctx: cancelledContext, inputs: validInputs, contains: "context canceled"},
		{name: "empty batch", ctx: t.Context, inputs: func() []search.EmbeddingInput { return nil }, contains: "batch is required"},
		{name: "oversized batch", ctx: t.Context, inputs: func() []search.EmbeddingInput {
			return make([]search.EmbeddingInput, maxBatchSize+1)
		}, contains: fmt.Sprintf("must not exceed %d items", maxBatchSize)},
		{name: "oversized request", ctx: t.Context, inputs: func() []search.EmbeddingInput {
			return []search.EmbeddingInput{{SourceRecordID: "record-1", Text: strings.Repeat("x", maxRequestBytes)}}
		}, contains: fmt.Sprintf("request exceeds %d bytes", maxRequestBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &httpClientStub{}
			embedder, err := NewEmbedder(Config{APIKey: "key", Model: "model", Client: client})
			if err != nil {
				t.Fatalf("NewEmbedder() error = %v", err)
			}
			_, err = embedder.Embed(test.ctx(), test.inputs())
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Embed() error = %v, want error containing %q", err, test.contains)
			}
			if client.calls != 0 {
				t.Errorf("HTTP calls = %d, want 0", client.calls)
			}
		})
	}
}

func TestEmbedderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(strings.Repeat("x", maxResponseBytes+1)))
	}))
	defer server.Close()

	_, err := newTestEmbedder(t, server).Embed(t.Context(), validInputs())
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("body exceeds %d bytes", maxResponseBytes)) {
		t.Fatalf("Embed() error = %v, want oversized response error", err)
	}
}

func TestEmbedderSanitizesProviderErrors(t *testing.T) {
	message := "quota\nexceeded " + strings.Repeat("private ", 100)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeResponse(t, response, http.StatusTooManyRequests, fmt.Sprintf(`{
			"error":{"message":%q,"type":"rate_limit_error","code":"rate_limit_exceeded"}
		}`, message))
	}))
	defer server.Close()

	_, err := newTestEmbedder(t, server).Embed(t.Context(), validInputs())
	if err == nil || !strings.Contains(err.Error(), "status 429: rate_limit_error: rate_limit_exceeded: quota exceeded") {
		t.Fatalf("Embed() error = %v, want structured provider error", err)
	}
	if strings.Contains(err.Error(), "\n") || len(err.Error()) > 400 || strings.Contains(err.Error(), "test-secret") {
		t.Errorf("Embed() exposed unsanitized provider data: %q", err)
	}
}

func TestEmbedderPreservesCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()
	embedder := newTestEmbedder(t, server)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := embedder.Embed(ctx, validInputs())
		result <- err
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		close(release)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "send OpenAI Embeddings API request") {
			t.Fatalf("Embed() error = %v, want contextual cancellation", err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("Embed() did not return after cancellation")
	}
}

func TestEmbedderPreservesRequestDeadline(t *testing.T) {
	client := &blockingHTTPClient{started: make(chan struct{})}
	embedder, err := NewEmbedder(Config{
		APIKey: "key", Model: "model", Client: client, RequestBudget: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	_, err = embedder.Embed(t.Context(), validInputs())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Embed() error = %v, want deadline exceeded", err)
	}
}

func newTestEmbedder(t *testing.T, server *httptest.Server) *Embedder {
	t.Helper()
	embedder, err := NewEmbedder(Config{
		APIKey:        "test-secret",
		Model:         "test-model",
		Client:        server.Client(),
		Endpoint:      server.URL + "/v1/embeddings",
		RequestBudget: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}
	return embedder
}

func validInputs() []search.EmbeddingInput {
	return []search.EmbeddingInput{{SourceRecordID: "record-1", Text: "Headline"}}
}

func writeResponse(t *testing.T, response http.ResponseWriter, status int, body string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if _, err := response.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

type httpClientStub struct {
	calls int
}

func (client *httpClientStub) Do(*http.Request) (*http.Response, error) {
	client.calls++
	return nil, errors.New("unexpected HTTP request")
}

type blockingHTTPClient struct {
	started chan struct{}
}

func (client *blockingHTTPClient) Do(request *http.Request) (*http.Response, error) {
	close(client.started)
	<-request.Context().Done()
	return nil, request.Context().Err()
}
