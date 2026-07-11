// Package sourcehttp retrieves bounded calendar-source documents over HTTP.
package sourcehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRequestBudget = 30 * time.Second
	maxResponseSize      = 10 << 20
	userAgent            = "Atlas (+https://github.com/Yanis897349/atlas)"
)

// ErrNegativeRequestBudget reports an invalid negative retrieval budget.
var ErrNegativeRequestBudget = errors.New("request budget must not be negative")

// Client is the HTTP operation used to retrieve a calendar source.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// Config describes one bounded calendar-source retrieval.
type Config struct {
	Resource      string
	URL           string
	Accept        string
	Client        Client
	RequestBudget time.Duration
}

// Fetcher retrieves one configured calendar-source document.
type Fetcher struct {
	resource      string
	url           string
	accept        string
	client        Client
	requestBudget time.Duration
}

// New validates config and returns a bounded calendar-source fetcher.
func New(config Config) (*Fetcher, error) {
	resource := strings.TrimSpace(config.Resource)
	if resource == "" {
		return nil, errors.New("calendar resource is required")
	}
	validatedURL, err := validateHTTPURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid %s URL: %w", resource, err)
	}
	accept := strings.TrimSpace(config.Accept)
	if accept == "" {
		return nil, errors.New("calendar Accept media type is required")
	}
	if config.RequestBudget < 0 {
		return nil, ErrNegativeRequestBudget
	}

	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: defaultRequestBudget}
	}
	requestBudget := config.RequestBudget
	if requestBudget == 0 {
		requestBudget = defaultRequestBudget
	}

	return &Fetcher{
		resource:      resource,
		url:           validatedURL,
		accept:        accept,
		client:        client,
		requestBudget: requestBudget,
	}, nil
}

// Fetch retrieves the configured document within its request and size bounds.
func (fetcher *Fetcher) Fetch(ctx context.Context) ([]byte, error) {
	return fetcher.fetch(ctx, nil)
}

// FetchWithQuery retrieves the configured document with the supplied query values.
func (fetcher *Fetcher) FetchWithQuery(ctx context.Context, query url.Values) ([]byte, error) {
	return fetcher.fetch(ctx, query)
}

func (fetcher *Fetcher) fetch(ctx context.Context, query url.Values) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetcher.requestBudget)
	defer cancel()

	requestURL, err := fetcher.requestURL(query)
	if err != nil {
		return nil, fmt.Errorf("create %s request URL: %w", fetcher.resource, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", fetcher.resource, err)
	}
	request.Header.Set("Accept", fetcher.accept)
	request.Header.Set("User-Agent", userAgent)

	response, err := fetcher.client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("fetch %s: %w", fetcher.resource, err)
	}
	if response == nil {
		return nil, fmt.Errorf("fetch %s: HTTP client returned a nil response", fetcher.resource)
	}
	if response.Body == nil {
		return nil, fmt.Errorf("fetch %s: HTTP response body is nil", fetcher.resource)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch %s: unexpected HTTP status %d", fetcher.resource, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fetcher.resource, err)
	}
	if len(body) > maxResponseSize {
		return nil, fmt.Errorf("read %s: response exceeds %d bytes", fetcher.resource, maxResponseSize)
	}
	return body, nil
}

func (fetcher *Fetcher) requestURL(query url.Values) (string, error) {
	if len(query) == 0 {
		return fetcher.url, nil
	}
	parsed, err := url.Parse(fetcher.url)
	if err != nil {
		return "", err
	}
	merged := parsed.Query()
	for key, values := range query {
		merged.Del(key)
		for _, value := range values {
			merged.Add(key, value)
		}
	}
	parsed.RawQuery = merged.Encode()
	return parsed.String(), nil
}

func validateHTTPURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", errors.New("must be an absolute HTTP(S) URL")
	}
	return parsed.String(), nil
}
