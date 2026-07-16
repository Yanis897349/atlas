package bls

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	maxRequestBytes  = 4 << 10
	maxResponseBytes = 1 << 20
	userAgent        = "Atlas (+https://github.com/Yanis897349/atlas)"
)

type apiRequest struct {
	SeriesIDs []Series `json:"seriesid"`
}

func (adapter *Adapter) fetch(ctx context.Context, targets []Target) ([]byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, adapter.requestBudget)
	defer cancel()
	if err := requestContext.Err(); err != nil {
		return nil, err
	}

	seriesIDs := make([]Series, len(targets))
	for index, target := range targets {
		seriesIDs[index] = target.Series
	}
	requestBody, err := json.Marshal(apiRequest{SeriesIDs: seriesIDs})
	if err != nil {
		return nil, fmt.Errorf("encode BLS API request: %w", err)
	}
	if len(requestBody) > maxRequestBytes {
		return nil, fmt.Errorf("encode BLS API request: body exceeds %d bytes", maxRequestBytes)
	}

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		adapter.endpoint,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create BLS API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", userAgent)

	response, err := adapter.client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("send BLS API request: %w", err)
	}
	if response == nil {
		return nil, errors.New("send BLS API request: HTTP client returned no response")
	}
	if response.Body == nil {
		return nil, errors.New("send BLS API request: HTTP response body is nil")
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read BLS API response: %w", err)
	}
	if len(responseBody) > maxResponseBytes {
		return nil, fmt.Errorf("read BLS API response: body exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("BLS API returned status %d", response.StatusCode)
	}
	return responseBody, nil
}
