package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryCreatesEventLinksIdempotentlyInFirstOccurrenceOrder(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Classified events", Symbols: []string{"DXY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	eventIDs := []string{
		"00000000-0000-0000-0000-000000000901",
		"00000000-0000-0000-0000-000000000902",
	}
	for index, eventID := range eventIDs {
		insertEconomicEvent(
			t,
			database.Pool,
			eventID,
			fmtExternalID(index),
			time.Date(2026, time.July, 20+index, 12, 0, 0, 0, time.FixedZone("test", 2*60*60)),
		)
	}

	existing, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "EURUSD", eventIDs[1], "original-actor",
	)
	if err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}
	classifications := []watchlist.EventRelevance{
		relevantClassification(" dxy ", eventIDs[0]),
		relevantClassification("eurusd", eventIDs[1]),
		relevantClassification("DXY", eventIDs[0]),
		relevantClassification(" EURUSD ", eventIDs[0]),
	}

	created, err := repository.CreateEventLinks(
		t.Context(), storedWatchlist.ID, classifications, "  classifier  ",
	)
	if err != nil {
		t.Fatalf("CreateEventLinks() error = %v", err)
	}
	assertBatchEventLinks(t, created, storedWatchlist.ID, []string{"DXY", "EURUSD", "EURUSD"}, []string{
		eventIDs[0], eventIDs[1], eventIDs[0],
	})
	if created[1] != existing {
		t.Errorf("existing link = %#v, want unchanged %#v", created[1], existing)
	}
	for _, link := range created {
		if link.ID == existing.ID {
			continue
		}
		if link.CreatedBy != "classifier" || link.UpdatedBy != "classifier" {
			t.Errorf("new link audit = (%q, %q), want classifier", link.CreatedBy, link.UpdatedBy)
		}
	}

	retried, err := repository.CreateEventLinks(
		t.Context(), storedWatchlist.ID, classifications, "retry-actor",
	)
	if err != nil {
		t.Fatalf("retry CreateEventLinks() error = %v", err)
	}
	if !reflect.DeepEqual(retried, created) {
		t.Errorf("retry links = %#v, want unchanged %#v", retried, created)
	}
	assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 3)
}

func TestRepositoryCreateEventLinksReturnsNonNilEmptyResult(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}

	links, err := repository.CreateEventLinks(
		t.Context(), "00000000-0000-0000-0000-000000000001", nil, "actor",
	)
	if err != nil {
		t.Fatalf("CreateEventLinks() error = %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("CreateEventLinks() = %#v, want non-nil empty result", links)
	}
}

func TestRepositoryValidatesEventLinkBatchBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	validID := "00000000-0000-0000-0000-000000000001"
	valid := relevantClassification("SPY", validID)

	tests := []struct {
		name            string
		watchlistID     string
		classifications []watchlist.EventRelevance
		actor           string
	}{
		{name: "watchlist ID", watchlistID: "bad", classifications: []watchlist.EventRelevance{valid}, actor: "actor"},
		{name: "actor", watchlistID: validID, classifications: []watchlist.EventRelevance{valid}, actor: " "},
		{name: "non-relevant classification", watchlistID: validID, classifications: []watchlist.EventRelevance{{Symbol: "SPY", Event: calendar.StoredEvent{ID: validID}}}, actor: "actor"},
		{name: "symbol", watchlistID: validID, classifications: []watchlist.EventRelevance{relevantClassification(" ", validID)}, actor: "actor"},
		{name: "event ID", watchlistID: validID, classifications: []watchlist.EventRelevance{relevantClassification("SPY", "bad")}, actor: "actor"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.CreateEventLinks(
				t.Context(), test.watchlistID, test.classifications, test.actor,
			); err == nil {
				t.Fatal("CreateEventLinks() error = nil, want validation error")
			}
		})
	}
}

func TestRepositoryCreateEventLinksRequiresExistingReferencesAndRollsBack(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "References", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000911"
	missingID := "00000000-0000-0000-0000-000000000919"
	insertEconomicEvent(t, database.Pool, eventID, "batch-reference", time.Now())

	tests := []struct {
		name        string
		watchlistID string
		batch       []watchlist.EventRelevance
	}{
		{name: "watchlist", watchlistID: missingID, batch: []watchlist.EventRelevance{relevantClassification("SPY", eventID)}},
		{name: "instrument", watchlistID: storedWatchlist.ID, batch: []watchlist.EventRelevance{relevantClassification("DXY", eventID)}},
		{name: "event", watchlistID: storedWatchlist.ID, batch: []watchlist.EventRelevance{relevantClassification("SPY", missingID)}},
		{name: "later event", watchlistID: storedWatchlist.ID, batch: []watchlist.EventRelevance{
			relevantClassification("SPY", eventID), relevantClassification("SPY", missingID),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.CreateEventLinks(t.Context(), test.watchlistID, test.batch, "classifier")
			if !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "create watchlist event link") {
				t.Fatalf("CreateEventLinks() error = %v, want contextual pgx.ErrNoRows", err)
			}
			assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 0)
		})
	}
}

func TestRepositoryCreateEventLinksPreservesCancellation(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	validID := "00000000-0000-0000-0000-000000000001"

	_, err := repository.CreateEventLinks(
		ctx, validID, []watchlist.EventRelevance{relevantClassification("SPY", validID)}, "actor",
	)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("CreateEventLinks() error = %v, want context.Canceled", err)
	}
}

func TestRepositoryCreateEventLinksRollsBackCommitFailure(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Deferred failure", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000921"
	insertEconomicEvent(t, database.Pool, eventID, "deferred-failure", time.Now())

	if _, err := database.Pool.Exec(t.Context(), `
CREATE FUNCTION reject_batch_event_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'reject batch event link';
END;
$$;
CREATE CONSTRAINT TRIGGER reject_batch_event_link
AFTER INSERT ON watchlist_event_links
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION reject_batch_event_link()
`); err != nil {
		t.Fatalf("create deferred event-link failure: %v", err)
	}

	_, err := repository.CreateEventLinks(
		t.Context(), storedWatchlist.ID,
		[]watchlist.EventRelevance{relevantClassification("SPY", eventID)},
		"classifier",
	)
	if err == nil || !strings.Contains(err.Error(), "commit watchlist event link creation") {
		t.Fatalf("CreateEventLinks() error = %v, want contextual commit failure", err)
	}
	assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 0)
}

func TestRepositoryCreateEventLinksHandlesConcurrentConflicts(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Concurrent classifications", Symbols: []string{"SPY"},
	}, "creator")
	eventIDs := []string{
		"00000000-0000-0000-0000-000000000931",
		"00000000-0000-0000-0000-000000000932",
	}
	for index, eventID := range eventIDs {
		insertEconomicEvent(t, database.Pool, eventID, fmtExternalID(index), time.Now())
	}
	if _, err := database.Pool.Exec(t.Context(), `
CREATE FUNCTION delay_batch_event_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_sleep(0.1);
    RETURN NEW;
END;
$$;
CREATE TRIGGER delay_batch_event_link
AFTER INSERT ON watchlist_event_links
FOR EACH ROW EXECUTE FUNCTION delay_batch_event_link()
`); err != nil {
		t.Fatalf("create event-link concurrency delay: %v", err)
	}

	batches := [][]watchlist.EventRelevance{
		{relevantClassification("SPY", eventIDs[0]), relevantClassification("SPY", eventIDs[1])},
		{relevantClassification("SPY", eventIDs[1]), relevantClassification("SPY", eventIDs[0])},
	}
	results := make(chan []watchlist.StoredEventLink, len(batches))
	errors := make(chan error, len(batches))
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for _, batch := range batches {
		waitGroup.Go(func() {
			<-start
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			links, err := repository.CreateEventLinks(ctx, storedWatchlist.ID, batch, "classifier")
			if err != nil {
				errors <- err
				return
			}
			results <- links
		})
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Errorf("concurrent CreateEventLinks() error = %v", err)
	}
	gotOrders := make(map[string]bool, len(batches))
	for links := range results {
		if len(links) != len(eventIDs) {
			t.Errorf("concurrent links = %#v, want two", links)
			continue
		}
		gotOrders[links[0].Event.ID+","+links[1].Event.ID] = true
	}
	for _, batch := range batches {
		order := batch[0].Event.ID + "," + batch[1].Event.ID
		if !gotOrders[order] {
			t.Errorf("concurrent results did not preserve input order %q", order)
		}
	}
	assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 2)
}

func relevantClassification(symbol string, eventID string) watchlist.EventRelevance {
	return watchlist.EventRelevance{
		Symbol: symbol,
		Event: calendar.StoredEvent{
			ID: eventID,
			Event: calendar.Event{
				ExternalEventID: "untrusted-input",
				Name:            "Untrusted input",
			},
		},
		Relevant: true,
	}
}

func assertBatchEventLinks(
	t *testing.T,
	links []watchlist.StoredEventLink,
	watchlistID string,
	symbols []string,
	eventIDs []string,
) {
	t.Helper()
	if len(links) != len(symbols) {
		t.Fatalf("link count = %d, want %d", len(links), len(symbols))
	}
	for index, link := range links {
		if !validUUID(link.ID) || link.WatchlistID != watchlistID ||
			link.Symbol != symbols[index] || link.Event.ID != eventIDs[index] {
			t.Errorf("link %d identity = %#v", index, link)
		}
		if link.Event.ExternalEventID == "untrusted-input" || link.Event.Name != "CPI release" ||
			link.Event.SourceURL == "" {
			t.Errorf("link %d event is not canonical = %#v", index, link.Event)
		}
		assertEventLinkUTC(t, link)
	}
}

func fmtExternalID(index int) string {
	if index == 0 {
		return "batch-first"
	}
	return "batch-second"
}
