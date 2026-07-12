package watchlistcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/watchlist"
)

func TestParseDoesNotRecognizeOtherCommands(t *testing.T) {
	for _, arguments := range [][]string{nil, {"migrate"}, {"daily-brief"}} {
		command, recognized, err := Parse(arguments)
		if err != nil || recognized || !reflect.DeepEqual(command, Command{}) {
			t.Errorf("Parse(%q) = (%#v, %t, %v), want zero command, false, nil", arguments, command, recognized, err)
		}
	}
}

func TestCommandReportsCandidateDependency(t *testing.T) {
	if !(Command{name: "link-watchlist-events"}).RequiresEventCandidates() {
		t.Error("link-watchlist-events must require event candidates")
	}
	if (Command{name: "watchlists"}).RequiresEventCandidates() {
		t.Error("watchlists must not require event candidates")
	}
}

func TestWatchlistDispatcherRoutesCandidateLinking(t *testing.T) {
	link := storedEventLinkFixture()
	candidates := &linkCandidateReaderStub{events: []calendar.StoredEvent{link.Event}}
	repository := &candidateDispatchRepository{
		reader: candidateWatchlistReaderStub{stored: watchlist.StoredWatchlist{
			ID:         link.WatchlistID,
			Definition: watchlist.Definition{Symbols: []string{"SPY"}},
		}},
		writer: candidateEventLinkWriterStub{links: []watchlist.StoredEventLink{link}},
	}
	stdout := &bytes.Buffer{}
	command := Command{
		name: "link-watchlist-events",
		linkEvents: linkWatchlistEventsCommand{
			watchlistID: link.WatchlistID,
			windowStart: link.Event.ScheduledAt.Add(-time.Hour),
			windowEnd:   link.Event.ScheduledAt.Add(time.Hour),
			limit:       10,
			actor:       "classifier",
		},
	}

	if err := Run(t.Context(), repository, candidates, stdout, command); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if candidates.calls != 1 || repository.reader.calls != 1 || repository.writer.calls != 1 {
		t.Errorf("dependency calls = (%d, %d, %d), want (1, 1, 1)",
			candidates.calls, repository.reader.calls, repository.writer.calls)
	}
	var output []watchlistEventOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output) != 1 || output[0].ID != link.ID {
		t.Errorf("output = %#v, want dispatched link %q", output, link.ID)
	}
}

func TestWatchlistDispatcherPreservesCandidateLinkingFailures(t *testing.T) {
	wantErr := errors.New("candidate repository unavailable")
	repository := &candidateDispatchRepository{}
	err := Run(t.Context(), repository, &linkCandidateReaderStub{err: wantErr}, &bytes.Buffer{},
		Command{name: "link-watchlist-events"})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "link watchlist event candidates") ||
		!strings.Contains(err.Error(), "retrieve watchlist event candidates") {
		t.Fatalf("Run() error = %v, want contextual %v", err, wantErr)
	}
	if repository.reader.calls != 0 || repository.writer.calls != 0 {
		t.Errorf("downstream calls = (%d, %d), want (0, 0)", repository.reader.calls, repository.writer.calls)
	}
}

type candidateDispatchRepository struct {
	Repository
	reader candidateWatchlistReaderStub
	writer candidateEventLinkWriterStub
}

func (repository *candidateDispatchRepository) Watchlist(
	ctx context.Context,
	id string,
) (watchlist.StoredWatchlist, error) {
	return repository.reader.Watchlist(ctx, id)
}

func (repository *candidateDispatchRepository) CreateEventLinks(
	ctx context.Context,
	watchlistID string,
	classifications []watchlist.EventRelevance,
	actor string,
) ([]watchlist.StoredEventLink, error) {
	return repository.writer.CreateEventLinks(ctx, watchlistID, classifications, actor)
}
