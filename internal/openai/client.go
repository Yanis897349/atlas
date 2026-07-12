// Package openai provides shared OpenAI HTTP client configuration and error handling.
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
	defaultRequestBudget = 30 * time.Second
	maxModelBytes        = 256
)

// HTTPClient executes OpenAI API requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config contains OpenAI client configuration shared by provider adapters.
type Config struct {
	APIKey        string
	Model         string
	Client        HTTPClient
	Endpoint      string
	RequestBudget time.Duration
}

// ResolvedConfig contains normalized, validated OpenAI client configuration.
type ResolvedConfig struct {
	APIKey        string
	Model         string
	Client        HTTPClient
	Endpoint      string
	RequestBudget time.Duration
}

// ResolveConfig validates config and applies adapter-specific endpoint defaults.
func ResolveConfig(config Config, defaultEndpoint string) (ResolvedConfig, error) {
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return ResolvedConfig{}, errors.New("OpenAI API key is required")
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		return ResolvedConfig{}, errors.New("OpenAI model is required")
	}
	if len(model) > maxModelBytes {
		return ResolvedConfig{}, fmt.Errorf("OpenAI model must not exceed %d bytes", maxModelBytes)
	}
	if config.RequestBudget < 0 {
		return ResolvedConfig{}, errors.New("OpenAI request budget must not be negative")
	}

	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") ||
		parsedEndpoint.Hostname() == "" || parsedEndpoint.User != nil || parsedEndpoint.Fragment != "" {
		return ResolvedConfig{}, errors.New(
			"OpenAI endpoint must be an absolute HTTP(S) URL without credentials or a fragment",
		)
	}
	if parsedEndpoint.Scheme != "https" && !isLoopbackHost(parsedEndpoint.Hostname()) {
		return ResolvedConfig{}, errors.New("OpenAI endpoint must use HTTPS unless it targets a loopback host")
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

	return ResolvedConfig{
		APIKey:        apiKey,
		Model:         model,
		Client:        client,
		Endpoint:      endpoint,
		RequestBudget: requestBudget,
	}, nil
}

func isLoopbackHost(host string) bool {
	if strings.TrimSuffix(strings.ToLower(host), ".") == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
