package openai

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResolveConfigValidatesSharedOpenAIConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing API key", config: Config{Model: "model"}, want: "API key is required"},
		{name: "missing model", config: Config{APIKey: "key"}, want: "model is required"},
		{name: "oversized model", config: Config{
			APIKey: "key", Model: strings.Repeat("m", maxModelBytes+1),
		}, want: "model must not exceed"},
		{name: "negative request budget", config: Config{
			APIKey: "key", Model: "model", RequestBudget: -time.Second,
		}, want: "request budget must not be negative"},
		{name: "relative endpoint", config: Config{
			APIKey: "key", Model: "model", Endpoint: "/v1/embeddings",
		}, want: "absolute HTTP(S) URL"},
		{name: "unsupported endpoint scheme", config: Config{
			APIKey: "key", Model: "model", Endpoint: "file:///tmp/embeddings",
		}, want: "absolute HTTP(S) URL"},
		{name: "remote plaintext endpoint", config: Config{
			APIKey: "key", Model: "model", Endpoint: "http://api.example.com/v1/embeddings",
		}, want: "must use HTTPS unless it targets a loopback host"},
		{name: "endpoint credentials", config: Config{
			APIKey: "key", Model: "model", Endpoint: "https://user@example.com/v1/embeddings",
		}, want: "without credentials or a fragment"},
		{name: "endpoint fragment", config: Config{
			APIKey: "key", Model: "model", Endpoint: "https://api.example.com/v1/embeddings#fragment",
		}, want: "without credentials or a fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := ResolveConfig(test.config, "https://api.example.com/v1/default")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ResolveConfig() = (%#v, %v), want error containing %q", resolved, err, test.want)
			}
		})
	}
}

func TestResolveConfigNormalizesDefaultsAndPreservesCustomClient(t *testing.T) {
	resolved, err := ResolveConfig(
		Config{APIKey: " key ", Model: " model "},
		"https://api.example.com/v1/default",
	)
	if err != nil {
		t.Fatalf("ResolveConfig() error = %v", err)
	}
	if resolved.APIKey != "key" || resolved.Model != "model" ||
		resolved.Endpoint != "https://api.example.com/v1/default" || resolved.RequestBudget != defaultRequestBudget {
		t.Errorf("ResolveConfig() = %#v", resolved)
	}
	client, ok := resolved.Client.(*http.Client)
	if !ok || client.Timeout != defaultRequestBudget || client.CheckRedirect == nil ||
		!errors.Is(client.CheckRedirect(&http.Request{}, nil), http.ErrUseLastResponse) {
		t.Errorf("default HTTP client must use the request budget and reject redirects")
	}

	customClient := &http.Client{}
	custom, err := ResolveConfig(Config{
		APIKey: "key", Model: "model", Client: customClient,
		Endpoint: "http://localhost./v1/test", RequestBudget: time.Second,
	}, "https://unused.example.com")
	if err != nil {
		t.Fatalf("ResolveConfig() custom error = %v", err)
	}
	if custom.Client != customClient || custom.Endpoint != "http://localhost./v1/test" ||
		custom.RequestBudget != time.Second {
		t.Errorf("ResolveConfig() custom = %#v", custom)
	}
}
