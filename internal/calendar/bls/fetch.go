package bls

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const (
	maxCalendarSize = 10 << 20
	userAgent       = "Atlas (+https://github.com/Yanis897349/atlas)"
)

func (adapter *Adapter) fetchBody(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, adapter.requestBudget)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, adapter.calendarURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create BLS calendar request: %w", err)
	}
	request.Header.Set("Accept", "text/calendar")
	request.Header.Set("User-Agent", userAgent)

	response, err := adapter.client.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("fetch BLS calendar: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("fetch BLS calendar: HTTP client returned a nil response")
	}
	if response.Body == nil {
		return nil, fmt.Errorf("fetch BLS calendar: HTTP response body is nil")
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch BLS calendar: unexpected HTTP status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCalendarSize+1))
	if err != nil {
		return nil, fmt.Errorf("read BLS calendar: %w", err)
	}
	if len(body) > maxCalendarSize {
		return nil, fmt.Errorf("read BLS calendar: response exceeds %d bytes", maxCalendarSize)
	}
	return body, nil
}
