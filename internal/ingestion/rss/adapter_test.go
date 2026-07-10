package rss_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion/rss"
)

func TestAdapterFetchNormalizesFixture(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 10, 12, 30, 0, 0, time.FixedZone("CEST", 2*60*60))

	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example-central-bank",
		FeedURL: "https://example.com/feed.xml",
		Client:  fixtureClient(t, "valid.xml"),
		Now:     func() time.Time { return retrievedAt },
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	records, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Fetch() returned %d records, want 2", len(records))
	}

	first := records[0]
	if first.Source != "example-central-bank" {
		t.Errorf("Source = %q, want %q", first.Source, "example-central-bank")
	}
	if first.SourceItemID != "af2da858076cf4d9f3730d7d522301b0c6a29be8ad212840ba9464ac595422ec" {
		t.Errorf("SourceItemID = %q, want stable GUID-derived ID", first.SourceItemID)
	}
	if first.OriginalURL != "https://example.com/releases/rates-july" {
		t.Errorf("OriginalURL = %q", first.OriginalURL)
	}
	if first.Title != "Policy rate unchanged" {
		t.Errorf("Title = %q", first.Title)
	}
	wantPublishedAt := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	if !first.PublishedAt.Equal(wantPublishedAt) {
		t.Errorf("PublishedAt = %v, want %v", first.PublishedAt, wantPublishedAt)
	}
	if !first.RetrievedAt.Equal(retrievedAt.UTC()) {
		t.Errorf("RetrievedAt = %v, want %v", first.RetrievedAt, retrievedAt.UTC())
	}

	if records[1].SourceItemID != "dc8223bcb18b5ac5f29834ba367e6037ba87ffb84b0d9b4fe6f9818d0e9e050c" {
		t.Errorf("fallback SourceItemID = %q, want stable URL-derived ID", records[1].SourceItemID)
	}
}

func TestAdapterFetchSupportsInvestingLiveFixture(t *testing.T) {
	retrievedAt := time.Date(2026, time.July, 10, 18, 0, 0, 0, time.UTC)
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  rss.InvestingLiveSource,
		FeedURL: rss.InvestingLiveFeedURL,
		Client:  fixtureClient(t, "investinglive.xml"),
		Now:     func() time.Time { return retrievedAt },
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	records, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Fetch() returned %d records, want 2", len(records))
	}

	first := records[0]
	if first.Source != rss.InvestingLiveSource {
		t.Errorf("Source = %q, want %q", first.Source, rss.InvestingLiveSource)
	}
	if first.Title != "Fed report highlights firmer spring inflation" {
		t.Errorf("Title = %q", first.Title)
	}
	if first.OriginalURL != "https://investinglive.com/central-banks/fed-spring-inflation/" {
		t.Errorf("OriginalURL = %q", first.OriginalURL)
	}
	wantPublishedAt := time.Date(2026, time.July, 10, 15, 6, 55, 0, time.UTC)
	if !first.PublishedAt.Equal(wantPublishedAt) {
		t.Errorf("PublishedAt = %v, want %v", first.PublishedAt, wantPublishedAt)
	}
	if first.SourceItemID == "" || first.SourceItemID == records[1].SourceItemID {
		t.Errorf("SourceItemID values are not unique: %q, %q", first.SourceItemID, records[1].SourceItemID)
	}
}

func TestAdapterFetchPreservesURLFragmentsAndNamedZoneOffsets(t *testing.T) {
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example-central-bank",
		FeedURL: "https://example.com/feed.xml",
		Client:  fixtureClient(t, "valid-edge-cases.xml"),
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	records, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Fetch() returned %d records, want 1", len(records))
	}

	record := records[0]
	if record.OriginalURL != "https://example.com/releases/report#details" {
		t.Errorf("OriginalURL = %q", record.OriginalURL)
	}
	wantPublishedAt := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	if !record.PublishedAt.Equal(wantPublishedAt) {
		t.Errorf("PublishedAt = %v, want %v", record.PublishedAt, wantPublishedAt)
	}
}

func TestAdapterFetchKeepsIdentityAcrossRepeatedIngestion(t *testing.T) {
	now := time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC)
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example-central-bank",
		FeedURL: "https://example.com/feed.xml",
		Client:  fixtureClient(t, "valid.xml"),
		Now:     func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	first, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("first Fetch() error = %v", err)
	}
	now = now.Add(time.Hour)
	second, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("second Fetch() error = %v", err)
	}

	for index := range first {
		if first[index].SourceItemID != second[index].SourceItemID {
			t.Errorf("SourceItemID changed from %q to %q", first[index].SourceItemID, second[index].SourceItemID)
		}
		if first[index].RetrievedAt.Equal(second[index].RetrievedAt) {
			t.Errorf("RetrievedAt did not change between fetches")
		}
	}
}

func TestAdapterFetchCollapsesRepeatedEntries(t *testing.T) {
	adapter, err := rss.NewAdapter(rss.Config{
		Source:  "example-central-bank",
		FeedURL: "https://example.com/feed.xml",
		Client:  fixtureClient(t, "repeated.xml"),
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	records, err := adapter.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Fetch() returned %d records, want 1", len(records))
	}
}

func TestAdapterFetchRejectsMalformedFixture(t *testing.T) {
	for _, fixture := range []string{"malformed.xml", "invalid-item.xml"} {
		t.Run(fixture, func(t *testing.T) {
			adapter, err := rss.NewAdapter(rss.Config{
				Source:  "example-central-bank",
				FeedURL: "https://example.com/feed.xml",
				Client:  fixtureClient(t, fixture),
			})
			if err != nil {
				t.Fatalf("NewAdapter() error = %v", err)
			}

			if records, err := adapter.Fetch(context.Background()); err == nil {
				t.Fatalf("Fetch() = %v, nil; want malformed input error", records)
			}
		})
	}
}

func TestNewAdapterValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config rss.Config
	}{
		{name: "missing source", config: rss.Config{FeedURL: "https://example.com/feed.xml"}},
		{name: "relative feed URL", config: rss.Config{Source: "source", FeedURL: "/feed.xml"}},
		{name: "unsupported feed URL scheme", config: rss.Config{Source: "source", FeedURL: "file:///feed.xml"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := rss.NewAdapter(test.config); err == nil {
				t.Fatal("NewAdapter() error = nil, want validation error")
			}
		})
	}
}

type staticClient struct {
	contents []byte
}

func (client staticClient) Do(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(client.contents)),
	}, nil
}

func fixtureClient(t *testing.T, name string) staticClient {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}

	return staticClient{contents: contents}
}
