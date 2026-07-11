package postgres

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/watchlist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRepositoryCreatesAndRetrievesEventLink(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Macro events", Symbols: []string{"EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	eventID := "00000000-0000-0000-0000-000000000101"
	scheduledAt := time.Date(2026, time.July, 15, 12, 30, 0, 0, time.FixedZone("test", 2*60*60))
	insertEconomicEvent(t, database.Pool, eventID, "cpi-july", scheduledAt)

	created, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, " eurusd ", eventID, "  analyst  ",
	)
	if err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}
	if !validUUID(created.ID) || created.WatchlistID != storedWatchlist.ID || created.Symbol != "EURUSD" {
		t.Errorf("CreateEventLink() identity = %#v", created)
	}
	if created.CreatedBy != "analyst" || created.UpdatedBy != "analyst" ||
		created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("CreateEventLink() audit = %#v", created)
	}
	if created.Event.ID != eventID || created.Event.Source != "test-calendar" ||
		created.Event.ExternalEventID != "cpi-july" || created.Event.Name != "CPI release" ||
		created.Event.Region != calendar.RegionUnitedStates || created.Event.Type != calendar.EventTypeInflation ||
		created.Event.SourceURL != "https://example.com/events/cpi-july" ||
		!created.Event.ScheduledAt.Equal(scheduledAt) {
		t.Errorf("CreateEventLink() event = %#v", created.Event)
	}
	assertEventLinkUTC(t, created)

	got, err := repository.EventLinks(t.Context(), storedWatchlist.ID, " eurUsd ", 10)
	if err != nil {
		t.Fatalf("EventLinks() error = %v", err)
	}
	if !reflect.DeepEqual(got, []watchlist.StoredEventLink{created}) {
		t.Errorf("EventLinks() = %#v, want %#v", got, []watchlist.StoredEventLink{created})
	}
}

func TestRepositoryEventLinksOrdersLimitsAndReturnsEmpty(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, err := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Ordered events", Symbols: []string{"SPY", "EURUSD"},
	}, "creator")
	if err != nil {
		t.Fatalf("CreateWatchlist() error = %v", err)
	}

	tieTime := time.Date(2026, time.July, 18, 14, 0, 0, 0, time.UTC)
	events := []struct {
		id          string
		externalID  string
		scheduledAt time.Time
	}{
		{id: "00000000-0000-0000-0000-000000000203", externalID: "earliest", scheduledAt: tieTime.Add(-time.Hour)},
		{id: "00000000-0000-0000-0000-000000000202", externalID: "tie-later-id", scheduledAt: tieTime},
		{id: "00000000-0000-0000-0000-000000000201", externalID: "tie-earlier-id", scheduledAt: tieTime},
	}
	for _, event := range events {
		insertEconomicEvent(t, database.Pool, event.id, event.externalID, event.scheduledAt)
		if _, err := repository.CreateEventLink(
			t.Context(), storedWatchlist.ID, "SPY", event.id, "analyst",
		); err != nil {
			t.Fatalf("CreateEventLink(%q) error = %v", event.externalID, err)
		}
	}

	got, err := repository.EventLinks(t.Context(), storedWatchlist.ID, "SPY", 2)
	if err != nil {
		t.Fatalf("EventLinks() error = %v", err)
	}
	wantIDs := []string{events[0].id, events[2].id}
	if eventLinkEventIDs(got) == nil || !reflect.DeepEqual(eventLinkEventIDs(got), wantIDs) {
		t.Errorf("EventLinks() event IDs = %v, want %v", eventLinkEventIDs(got), wantIDs)
	}

	empty, err := repository.EventLinks(t.Context(), storedWatchlist.ID, "EURUSD", 10)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("EventLinks() empty result = (%#v, %v), want non-nil empty slice", empty, err)
	}
}

func TestRepositoryRejectsDuplicateEventLinkWithoutChangingOriginal(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Duplicates", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000301"
	insertEconomicEvent(t, database.Pool, eventID, "duplicate", time.Now())

	original, err := repository.CreateEventLink(t.Context(), storedWatchlist.ID, "SPY", eventID, "first")
	if err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}
	_, err = repository.CreateEventLink(t.Context(), storedWatchlist.ID, "SPY", eventID, "second")
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23505" ||
		!strings.Contains(err.Error(), "create watchlist event link") {
		t.Fatalf("duplicate CreateEventLink() error = %v, want contextual unique violation", err)
	}

	links, err := repository.EventLinks(t.Context(), storedWatchlist.ID, "SPY", 10)
	if err != nil {
		t.Fatalf("EventLinks() error = %v", err)
	}
	if !reflect.DeepEqual(links, []watchlist.StoredEventLink{original}) {
		t.Errorf("EventLinks() = %#v, want unchanged original %#v", links, original)
	}
}

func TestRepositoryEventLinksRequireExistingReferences(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "References", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000401"
	insertEconomicEvent(t, database.Pool, eventID, "references", time.Now())
	missingID := "00000000-0000-0000-0000-000000000499"

	creationTests := []struct {
		name        string
		watchlistID string
		symbol      string
		eventID     string
	}{
		{name: "watchlist", watchlistID: missingID, symbol: "SPY", eventID: eventID},
		{name: "instrument", watchlistID: storedWatchlist.ID, symbol: "EURUSD", eventID: eventID},
		{name: "event", watchlistID: storedWatchlist.ID, symbol: "SPY", eventID: missingID},
	}
	for _, test := range creationTests {
		t.Run("missing "+test.name, func(t *testing.T) {
			_, err := repository.CreateEventLink(
				t.Context(), test.watchlistID, test.symbol, test.eventID, "analyst",
			)
			if !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "create watchlist event link") {
				t.Fatalf("CreateEventLink() error = %v, want contextual pgx.ErrNoRows", err)
			}
		})
	}

	for _, test := range []struct {
		name        string
		watchlistID string
		symbol      string
	}{
		{name: "watchlist", watchlistID: missingID, symbol: "SPY"},
		{name: "instrument", watchlistID: storedWatchlist.ID, symbol: "EURUSD"},
	} {
		t.Run("retrieve missing "+test.name, func(t *testing.T) {
			_, err := repository.EventLinks(t.Context(), test.watchlistID, test.symbol, 10)
			if !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "resolve watchlist instrument") {
				t.Fatalf("EventLinks() error = %v, want contextual pgx.ErrNoRows", err)
			}
		})
	}
}

func TestRepositoryEventLinksCascadeWithInstrumentAndWatchlist(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	eventID := "00000000-0000-0000-0000-000000000501"
	insertEconomicEvent(t, database.Pool, eventID, "cascade", time.Now())

	updatedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Updated", Symbols: []string{"SPY"},
	}, "creator")
	if _, err := repository.CreateEventLink(
		t.Context(), updatedWatchlist.ID, "SPY", eventID, "analyst",
	); err != nil {
		t.Fatalf("CreateEventLink() before update error = %v", err)
	}
	if _, err := repository.UpdateWatchlist(t.Context(), updatedWatchlist.ID, watchlist.Definition{
		Name: "Updated", Symbols: []string{"SPY"},
	}, "editor"); err != nil {
		t.Fatalf("UpdateWatchlist() error = %v", err)
	}
	assertEventLinkCount(t, database.Pool, updatedWatchlist.ID, 0)

	deletedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Deleted", Symbols: []string{"SPY"},
	}, "creator")
	if _, err := repository.CreateEventLink(
		t.Context(), deletedWatchlist.ID, "SPY", eventID, "analyst",
	); err != nil {
		t.Fatalf("CreateEventLink() before delete error = %v", err)
	}
	if err := repository.DeleteWatchlist(t.Context(), deletedWatchlist.ID); err != nil {
		t.Fatalf("DeleteWatchlist() error = %v", err)
	}
	assertEventLinkCount(t, database.Pool, deletedWatchlist.ID, 0)
}

func TestWatchlistEventLinkSchemaPreservesReferencedEvents(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	storedWatchlist, _ := repository.CreateWatchlist(t.Context(), watchlist.Definition{
		Name: "Traceability", Symbols: []string{"SPY"},
	}, "creator")
	eventID := "00000000-0000-0000-0000-000000000601"
	insertEconomicEvent(t, database.Pool, eventID, "traceability", time.Now())
	if _, err := repository.CreateEventLink(
		t.Context(), storedWatchlist.ID, "SPY", eventID, "analyst",
	); err != nil {
		t.Fatalf("CreateEventLink() error = %v", err)
	}

	if _, err := database.Pool.Exec(t.Context(), `DELETE FROM economic_events WHERE id = $1`, eventID); err == nil {
		t.Fatal("deleting a linked economic event succeeded")
	}

	var instrumentID string
	if err := database.Pool.QueryRow(t.Context(), `
SELECT id::text FROM watchlist_instruments WHERE watchlist_id = $1 AND symbol = 'SPY'
`, storedWatchlist.ID).Scan(&instrumentID); err != nil {
		t.Fatalf("query watchlist instrument: %v", err)
	}
	secondEventID := "00000000-0000-0000-0000-000000000602"
	insertEconomicEvent(t, database.Pool, secondEventID, "schema-audit", time.Now())
	if _, err := database.Pool.Exec(t.Context(), `
INSERT INTO watchlist_event_links (watchlist_instrument_id, economic_event_id, created_by, updated_by)
	VALUES ($1, $2, ' ', 'user')
`, instrumentID, secondEventID); err == nil {
		t.Fatal("blank link creator insert succeeded")
	}
	if _, err := database.Pool.Exec(t.Context(), `
INSERT INTO watchlist_event_links (watchlist_instrument_id, economic_event_id, created_by, updated_by)
VALUES ($1, $2, 'user', ' ')
`, instrumentID, secondEventID); err == nil {
		t.Fatal("blank link updater insert succeeded")
	}
	if _, err := database.Pool.Exec(t.Context(), `
INSERT INTO watchlist_event_links (watchlist_instrument_id, economic_event_id, created_by, updated_by)
VALUES ($1, '00000000-0000-0000-0000-000000000699', 'user', 'user')
`, instrumentID); err == nil {
		t.Fatal("missing economic event reference insert succeeded")
	}
	assertEventLinkCount(t, database.Pool, storedWatchlist.ID, 1)
}

func TestRepositoryValidatesEventLinksBeforePostgreSQL(t *testing.T) {
	repository, err := NewRepository(panicDB{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	validID := "00000000-0000-0000-0000-000000000001"

	creationTests := []struct {
		name        string
		watchlistID string
		symbol      string
		eventID     string
		actor       string
	}{
		{name: "watchlist ID", watchlistID: "bad", symbol: "SPY", eventID: validID, actor: "user"},
		{name: "symbol", watchlistID: validID, symbol: " ", eventID: validID, actor: "user"},
		{name: "event ID", watchlistID: validID, symbol: "SPY", eventID: "bad", actor: "user"},
		{name: "actor", watchlistID: validID, symbol: "SPY", eventID: validID, actor: " "},
	}
	for _, test := range creationTests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := repository.CreateEventLink(
				t.Context(), test.watchlistID, test.symbol, test.eventID, test.actor,
			); err == nil {
				t.Fatal("CreateEventLink() error = nil, want validation error")
			}
		})
	}

	for _, test := range []struct {
		name        string
		watchlistID string
		symbol      string
		limit       int
	}{
		{name: "watchlist ID", watchlistID: "bad", symbol: "SPY", limit: 10},
		{name: "symbol", watchlistID: validID, symbol: " ", limit: 10},
		{name: "zero limit", watchlistID: validID, symbol: "SPY", limit: 0},
		{name: "high limit", watchlistID: validID, symbol: "SPY", limit: watchlist.MaxEventLinksLimit + 1},
	} {
		t.Run("query "+test.name, func(t *testing.T) {
			if _, err := repository.EventLinks(
				t.Context(), test.watchlistID, test.symbol, test.limit,
			); err == nil {
				t.Fatal("EventLinks() error = nil, want validation error")
			}
		})
	}
}

func TestRepositoryEventLinksPreserveCancellation(t *testing.T) {
	database := openDatabase(t)
	repository, _ := NewRepository(database.Pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	validID := "00000000-0000-0000-0000-000000000001"

	if _, err := repository.CreateEventLink(ctx, validID, "SPY", validID, "user"); !errors.Is(err, context.Canceled) {
		t.Errorf("CreateEventLink() error = %v, want context.Canceled", err)
	}
	if _, err := repository.EventLinks(ctx, validID, "SPY", 10); !errors.Is(err, context.Canceled) {
		t.Errorf("EventLinks() error = %v, want context.Canceled", err)
	}
}

func insertEconomicEvent(
	t *testing.T,
	database interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	id string,
	externalID string,
	scheduledAt time.Time,
) {
	t.Helper()
	_, err := database.Exec(t.Context(), `
INSERT INTO economic_events (
    id, source, external_event_id, name, region, event_type, scheduled_at,
    source_url, retrieved_at, created_by, updated_by
)
VALUES ($1, 'test-calendar', $2, 'CPI release', 'united_states', 'inflation', $3,
        $4, $5, 'calendar-user', 'calendar-user')
`, id, externalID, scheduledAt, "https://example.com/events/"+externalID, scheduledAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("insert economic event: %v", err)
	}
}

func assertEventLinkCount(
	t *testing.T,
	database interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	watchlistID string,
	want int,
) {
	t.Helper()
	var got int
	if err := database.QueryRow(t.Context(), `
SELECT count(*)
FROM watchlist_event_links AS link
JOIN watchlist_instruments AS instrument ON instrument.id = link.watchlist_instrument_id
WHERE instrument.watchlist_id = $1
`, watchlistID).Scan(&got); err != nil {
		t.Fatalf("count watchlist event links: %v", err)
	}
	if got != want {
		t.Errorf("watchlist event link count = %d, want %d", got, want)
	}
}

func assertEventLinkUTC(t *testing.T, link watchlist.StoredEventLink) {
	t.Helper()
	timestamps := []time.Time{
		link.CreatedAt,
		link.UpdatedAt,
		link.Event.ScheduledAt,
		link.Event.RetrievedAt,
		link.Event.CreatedAt,
		link.Event.UpdatedAt,
	}
	for _, timestamp := range timestamps {
		if timestamp.Location() != time.UTC {
			t.Errorf("timestamp location = %v, want UTC", timestamp.Location())
		}
	}
}

func eventLinkEventIDs(links []watchlist.StoredEventLink) []string {
	ids := make([]string, len(links))
	for index, link := range links {
		ids[index] = link.Event.ID
	}
	return ids
}
