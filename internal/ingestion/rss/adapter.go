// Package rss ingests RSS 2.0 feeds into normalized source records.
package rss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

const (
	maxFeedSize          = 10 << 20
	defaultRequestBudget = 30 * time.Second

	// InvestingLiveSource is the normalized name of the initial Atlas RSS source.
	InvestingLiveSource = "investinglive"
	// InvestingLiveFeedURL is the canonical InvestingLive RSS endpoint.
	InvestingLiveFeedURL = "https://investinglive.com/feed/"
)

// HTTPClient is the subset of http.Client used to retrieve a feed.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Config identifies an RSS feed and its normalized source name.
type Config struct {
	Source        string
	FeedURL       string
	Client        HTTPClient
	Now           func() time.Time
	RequestBudget time.Duration
	Wait          func(context.Context, time.Duration) error
}

// Adapter fetches and normalizes one configured RSS feed.
type Adapter struct {
	source        string
	feedURL       string
	client        HTTPClient
	now           func() time.Time
	requestBudget time.Duration
	wait          func(context.Context, time.Duration) error
}

// NewAdapter validates config and returns an RSS adapter.
func NewAdapter(config Config) (*Adapter, error) {
	source := strings.TrimSpace(config.Source)
	if source == "" {
		return nil, errors.New("RSS source is required")
	}

	feedURL, err := validateHTTPURL(config.FeedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid RSS feed URL: %w", err)
	}

	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: defaultRequestBudget}
	}

	now := config.Now
	if now == nil {
		now = time.Now
	}
	requestBudget := config.RequestBudget
	if requestBudget < 0 {
		return nil, errors.New("RSS request budget must not be negative")
	}
	if requestBudget == 0 {
		requestBudget = defaultRequestBudget
	}
	wait := config.Wait
	if wait == nil {
		wait = waitForRetry
	}

	return &Adapter{
		source:        source,
		feedURL:       feedURL,
		client:        client,
		now:           now,
		requestBudget: requestBudget,
		wait:          wait,
	}, nil
}

// Fetch retrieves the configured feed and returns one record per unique item.
func (a *Adapter) Fetch(ctx context.Context) ([]ingestion.SourceRecord, error) {
	body, err := a.fetchBody(ctx)
	if err != nil {
		return nil, err
	}

	var document rssDocument
	if err := xml.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("parse RSS feed: %w", err)
	}

	retrievedAt := a.now().UTC()
	records := make([]ingestion.SourceRecord, 0, len(document.Channel.Items))
	seen := make(map[string]struct{}, len(document.Channel.Items))
	for index, item := range document.Channel.Items {
		record, err := a.normalize(item, retrievedAt)
		if err != nil {
			return nil, fmt.Errorf("normalize RSS item %d: %w", index+1, err)
		}
		if _, exists := seen[record.SourceItemID]; exists {
			continue
		}

		seen[record.SourceItemID] = struct{}{}
		records = append(records, record)
	}

	return records, nil
}

func (a *Adapter) normalize(item rssItem, retrievedAt time.Time) (ingestion.SourceRecord, error) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return ingestion.SourceRecord{}, errors.New("title is required")
	}

	originalURL, err := validateHTTPURL(item.Link)
	if err != nil {
		return ingestion.SourceRecord{}, fmt.Errorf("invalid item URL: %w", err)
	}

	publishedAt, err := parsePublicationTime(item.PubDate)
	if err != nil {
		return ingestion.SourceRecord{}, err
	}

	identity := strings.TrimSpace(item.GUID)
	if identity == "" {
		identity = originalURL
	}

	return ingestion.SourceRecord{
		Source:       a.source,
		SourceItemID: sourceItemID(a.source, identity),
		OriginalURL:  originalURL,
		Title:        title,
		PublishedAt:  publishedAt,
		RetrievedAt:  retrievedAt,
	}, nil
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

func sourceItemID(source, identity string) string {
	digest := sha256.Sum256([]byte(source + "\x00" + identity))
	return hex.EncodeToString(digest[:])
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	GUID    string `xml:"guid"`
	Link    string `xml:"link"`
	Title   string `xml:"title"`
	PubDate string `xml:"pubDate"`
}
