package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
)

func TestParseStoredDailyBriefsQuery(t *testing.T) {
	query, err := parseStoredDailyBriefsQuery([]string{
		"--limit", "12",
		"--to", "2026-07-11T14:30:00+02:00",
		"--region", "united_states",
		"--from", "2026-07-11T08:00:00-04:00",
	})
	if err != nil {
		t.Fatalf("parseStoredDailyBriefsQuery() error = %v", err)
	}
	if query.region != calendar.RegionUnitedStates || query.limit != 12 {
		t.Errorf("query classification = (%q, %d), want (%q, 12)", query.region, query.limit, calendar.RegionUnitedStates)
	}
	if !query.windowStart.Equal(time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)) ||
		!query.windowEnd.Equal(time.Date(2026, time.July, 11, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("query window = (%v, %v), want parsed offset instants", query.windowStart, query.windowEnd)
	}
}

func TestParseStoredDailyBriefsQueryAcceptsEqualInclusiveWindow(t *testing.T) {
	arguments := validStoredDailyBriefsArguments()
	arguments = replaceFlag(arguments, "--to", "2026-07-11T08:00:00Z")
	if _, err := parseStoredDailyBriefsQuery(arguments[1:]); err != nil {
		t.Fatalf("parseStoredDailyBriefsQuery() error = %v", err)
	}
}

func TestRunRejectsInvalidStoredDailyBriefsArgumentsBeforeDatabaseSetup(t *testing.T) {
	valid := validStoredDailyBriefsArguments()
	tests := []struct {
		name      string
		arguments []string
		contains  string
	}{
		{name: "missing region", arguments: withoutFlag(valid, "--region"), contains: "--region is required"},
		{name: "missing from", arguments: withoutFlag(valid, "--from"), contains: "--from is required"},
		{name: "missing to", arguments: withoutFlag(valid, "--to"), contains: "--to is required"},
		{name: "missing limit", arguments: withoutFlag(valid, "--limit"), contains: "--limit is required"},
		{name: "unknown flag", arguments: append(append([]string(nil), valid...), "--format", "yaml"), contains: "flag provided but not defined"},
		{name: "positional argument", arguments: append(append([]string(nil), valid...), "extra"), contains: "unexpected positional arguments"},
		{name: "unsupported region", arguments: replaceFlag(valid, "--region", "asia"), contains: "unsupported region"},
		{name: "malformed from", arguments: replaceFlag(valid, "--from", "today"), contains: "--from must be RFC3339"},
		{name: "malformed to", arguments: replaceFlag(valid, "--to", "today"), contains: "--to must be RFC3339"},
		{name: "reversed window", arguments: replaceFlag(valid, "--to", "2026-07-11T07:59:59Z"), contains: "--to must not be before --from"},
		{name: "zero limit", arguments: replaceFlag(valid, "--limit", "0"), contains: "--limit must be between 1 and 100"},
		{name: "limit above maximum", arguments: replaceFlag(valid, "--limit", "101"), contains: "--limit must be between 1 and 100"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Run(t.Context(), test.arguments, Dependencies{Getenv: func(string) string {
				t.Fatal("configuration read for invalid command arguments")
				return ""
			}})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Run() error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestRunStoredDailyBriefsWritesCompleteJSON(t *testing.T) {
	brief := storedDailyBriefOutputFixture()
	repository := &storedDailyBriefsReaderStub{briefs: []dailybrief.StoredBrief{brief}}
	stdout := &bytes.Buffer{}
	query := validStoredDailyBriefsQuery()

	if err := runStoredDailyBriefs(t.Context(), repository, stdout, query); err != nil {
		t.Fatalf("runStoredDailyBriefs() error = %v", err)
	}

	var output []dailyBriefOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if repository.calls != 1 || repository.region != query.region ||
		!repository.windowStart.Equal(query.windowStart) || !repository.windowEnd.Equal(query.windowEnd) ||
		repository.limit != query.limit {
		t.Errorf("repository query = (%d, %q, %v, %v, %d), want one complete query", repository.calls, repository.region, repository.windowStart, repository.windowEnd, repository.limit)
	}
	if len(output) != 1 || output[0].ID != brief.ID || output[0].Provider != "openai" ||
		output[0].Model != "test-model" || len(output[0].Sections) != 2 ||
		len(output[0].Sections[0].Citations) != 2 {
		t.Fatalf("output = %#v, want complete stored brief", output)
	}
	if output[0].Sections[0].Heading != "First & foremost" ||
		output[0].Sections[0].Citations[0].Kind != dailybrief.CitationUpcomingEvent ||
		output[0].Sections[0].Citations[1].Kind != dailybrief.CitationSourceRecord {
		t.Errorf("ordered sections and citations = %#v, want stored order", output[0].Sections)
	}
	if !strings.Contains(stdout.String(), "First & foremost") || !strings.HasSuffix(output[0].CreatedAt, "Z") ||
		output[0].PublicationWindow.From != "2026-07-11T08:00:00Z" {
		t.Errorf("serialized output = %q, want unescaped content and UTC timestamps", stdout.String())
	}
}

func TestRunStoredDailyBriefsWritesEmptyArray(t *testing.T) {
	stdout := &bytes.Buffer{}
	if err := runStoredDailyBriefs(t.Context(), &storedDailyBriefsReaderStub{}, stdout, validStoredDailyBriefsQuery()); err != nil {
		t.Fatalf("runStoredDailyBriefs() error = %v", err)
	}
	if stdout.String() != "[]\n" {
		t.Errorf("stdout = %q, want empty JSON array", stdout.String())
	}
}

func TestRunStoredDailyBriefsPreservesFailures(t *testing.T) {
	repositoryErr := errors.New("briefs unavailable")
	for _, test := range []struct {
		name       string
		repository dailybrief.Reader
		stdout     *errorWriter
		wantErr    error
		contains   string
	}{
		{name: "repository cancellation", repository: &storedDailyBriefsReaderStub{err: context.Canceled}, wantErr: context.Canceled, contains: "retrieve stored daily briefs"},
		{name: "repository failure", repository: &storedDailyBriefsReaderStub{err: repositoryErr}, wantErr: repositoryErr, contains: "retrieve stored daily briefs"},
		{name: "writer failure", repository: &storedDailyBriefsReaderStub{}, stdout: &errorWriter{err: repositoryErr}, wantErr: repositoryErr, contains: "encode stored daily briefs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout io.Writer = &bytes.Buffer{}
			if test.stdout != nil {
				stdout = test.stdout
			}
			err := runStoredDailyBriefs(t.Context(), test.repository, stdout, validStoredDailyBriefsQuery())
			if err == nil || !errors.Is(err, test.wantErr) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("runStoredDailyBriefs() error = %v, want wrapped %v with %q", err, test.wantErr, test.contains)
			}
		})
	}
}

func validStoredDailyBriefsArguments() []string {
	return []string{
		"daily-briefs",
		"--region", "united_states",
		"--from", "2026-07-11T08:00:00Z",
		"--to", "2026-07-11T12:00:00Z",
		"--limit", "10",
	}
}

func validStoredDailyBriefsQuery() storedDailyBriefsQuery {
	return storedDailyBriefsQuery{
		region:      calendar.RegionUnitedStates,
		windowStart: time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
		windowEnd:   time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
		limit:       10,
	}
}

func storedDailyBriefOutputFixture() dailybrief.StoredBrief {
	brief := dailybrief.Brief{
		Region:                 calendar.RegionUnitedStates,
		PublicationWindowStart: time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
		PublicationWindowEnd:   time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC),
		EventWindowStart:       time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC),
		EventWindowEnd:         time.Date(2026, time.July, 13, 8, 0, 0, 0, time.UTC),
		Provider:               "openai",
		Model:                  "test-model",
		Sections: []dailybrief.Section{
			{Heading: "First & foremost", Content: "First section", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationUpcomingEvent, ID: "00000000-0000-0000-0000-000000000002", Source: "official-calendar", URL: "https://example.com/event"},
				{Kind: dailybrief.CitationSourceRecord, ID: "00000000-0000-0000-0000-000000000001", Source: "example-news", URL: "https://example.com/news"},
			}},
			{Heading: "Second", Content: "Second section", Citations: []dailybrief.Citation{
				{Kind: dailybrief.CitationSourceRecord, ID: "00000000-0000-0000-0000-000000000001", Source: "example-news", URL: "https://example.com/news"},
			}},
		},
	}
	createdAt := time.Date(2026, time.July, 11, 14, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	return dailybrief.StoredBrief{
		ID:        "00000000-0000-0000-0000-000000000003",
		Brief:     brief,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		CreatedBy: "brief-worker",
		UpdatedBy: "brief-worker",
	}
}

type storedDailyBriefsReaderStub struct {
	briefs      []dailybrief.StoredBrief
	err         error
	calls       int
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

func (repository *storedDailyBriefsReaderStub) StoredDailyBriefs(
	_ context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]dailybrief.StoredBrief, error) {
	repository.calls++
	repository.region = region
	repository.windowStart = windowStart
	repository.windowEnd = windowEnd
	repository.limit = limit
	return repository.briefs, repository.err
}
