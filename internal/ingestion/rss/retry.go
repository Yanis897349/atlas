package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxFetchAttempts  = 3
	initialRetryDelay = time.Second
)

func (a *Adapter) fetchBody(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, a.requestBudget)
	defer cancel()

	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		response, err := a.fetch(ctx)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, fmt.Errorf("fetch RSS feed: %w", contextErr)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("fetch RSS feed: %w", err)
			}
			if attempt == maxFetchAttempts {
				return nil, fmt.Errorf("fetch RSS feed after %d attempts: %w", attempt, err)
			}
			if err := a.waitBeforeRetry(ctx, retryDelay(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			body, readErr := readFeedBody(response)
			if readErr != nil {
				return nil, readErr
			}
			return body, nil
		}

		statusErr := fmt.Errorf("unexpected HTTP status %s", response.Status)
		retryable := response.StatusCode == http.StatusTooManyRequests ||
			(response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= 599)
		delay, hasRetryAfter := retryAfterDelay(response.Header.Get("Retry-After"), a.now())
		_ = response.Body.Close()
		if !retryable {
			return nil, fmt.Errorf("fetch RSS feed: %w", statusErr)
		}
		if attempt == maxFetchAttempts {
			return nil, fmt.Errorf("fetch RSS feed after %d attempts: %w", attempt, statusErr)
		}
		if !hasRetryAfter {
			delay = retryDelay(attempt)
		}
		if err := a.waitBeforeRetry(ctx, delay); err != nil {
			return nil, err
		}
	}

	panic("bounded RSS fetch loop exhausted")
}

func (a *Adapter) fetch(ctx context.Context) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create RSS request: %w", err)
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8")

	response, err := a.client.Do(request)
	if err != nil && response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return response, err
}

func readFeedBody(response *http.Response) ([]byte, error) {
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxFeedSize+1))
	if err != nil {
		return nil, fmt.Errorf("read RSS feed: %w", err)
	}
	if len(body) > maxFeedSize {
		return nil, fmt.Errorf("read RSS feed: response exceeds %d bytes", maxFeedSize)
	}
	return body, nil
}

func (a *Adapter) waitBeforeRetry(ctx context.Context, delay time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok && delay > time.Until(deadline) {
		return fmt.Errorf("fetch RSS feed: retry delay exceeds request budget: %w", context.DeadlineExceeded)
	}
	if err := a.wait(ctx, delay); err != nil {
		return fmt.Errorf("fetch RSS feed: wait to retry: %w", err)
	}
	return nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(attempt int) time.Duration {
	return initialRetryDelay << (attempt - 1)
}

func retryAfterDelay(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 31); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil || when.Before(now) {
		return 0, false
	}
	return when.Sub(now), true
}
