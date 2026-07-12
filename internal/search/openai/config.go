package openai

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultEndpoint      = "https://api.openai.com/v1/embeddings"
	defaultRequestBudget = 30 * time.Second
	maxModelBytes        = 256
)

// Config configures an OpenAI source-record embedder.
type Config struct {
	APIKey        string
	Model         string
	Client        HTTPClient
	Endpoint      string
	RequestBudget time.Duration
}

// NewEmbedder returns a validated OpenAI source-record embedder.
func NewEmbedder(config Config) (*Embedder, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		return nil, errors.New("OpenAI model is required")
	}
	if len(model) > maxModelBytes {
		return nil, fmt.Errorf("OpenAI model must not exceed %d bytes", maxModelBytes)
	}
	if config.RequestBudget < 0 {
		return nil, errors.New("OpenAI request budget must not be negative")
	}

	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") ||
		parsedEndpoint.Hostname() == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" {
		return nil, errors.New("OpenAI endpoint must be an absolute HTTP(S) URL without credentials or a fragment")
	}
	if parsedEndpoint.Scheme != "https" && !isLoopbackHost(parsedEndpoint.Hostname()) {
		return nil, errors.New("OpenAI endpoint must use HTTPS unless it targets a loopback host")
	}

	requestBudget := config.RequestBudget
	if requestBudget == 0 {
		requestBudget = defaultRequestBudget
	}
	client := config.Client
	if client == nil {
		client = &http.Client{
			Timeout: requestBudget,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	return &Embedder{
		apiKey:        apiKey,
		model:         model,
		client:        client,
		endpoint:      endpoint,
		requestBudget: requestBudget,
	}, nil
}

func isLoopbackHost(host string) bool {
	if strings.TrimSuffix(strings.ToLower(host), ".") == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
