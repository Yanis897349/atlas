package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxStoredDailyBriefsLimit = 100

type dailyBriefPostgresDB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type dailyBriefRepository struct {
	db dailyBriefPostgresDB
}

type storedDailyBrief struct {
	ID string
	dailyBrief
	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
}

type dailyBriefPersistence interface {
	PersistDailyBrief(context.Context, dailyBrief, string) (storedDailyBrief, error)
}

func newDailyBriefRepository(db dailyBriefPostgresDB) (*dailyBriefRepository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &dailyBriefRepository{db: db}, nil
}

func (repository *dailyBriefRepository) PersistDailyBrief(
	ctx context.Context,
	brief dailyBrief,
	actor string,
) (storedDailyBrief, error) {
	actor = strings.TrimSpace(actor)
	brief.provider = strings.TrimSpace(brief.provider)
	brief.model = strings.TrimSpace(brief.model)
	if err := validateDailyBriefPersistence(brief, actor); err != nil {
		return storedDailyBrief{}, err
	}

	brief.publicationWindowStart = brief.publicationWindowStart.UTC()
	brief.publicationWindowEnd = brief.publicationWindowEnd.UTC()
	brief.eventWindowStart = brief.eventWindowStart.UTC()
	brief.eventWindowEnd = brief.eventWindowEnd.UTC()

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return storedDailyBrief{}, fmt.Errorf("begin daily brief persistence: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	stored := storedDailyBrief{dailyBrief: brief}
	if err := transaction.QueryRow(
		ctx,
		insertDailyBriefSQL,
		brief.region,
		brief.publicationWindowStart,
		brief.publicationWindowEnd,
		brief.eventWindowStart,
		brief.eventWindowEnd,
		brief.provider,
		brief.model,
		actor,
	).Scan(
		&stored.ID,
		&stored.CreatedAt,
		&stored.UpdatedAt,
		&stored.CreatedBy,
		&stored.UpdatedBy,
	); err != nil {
		return storedDailyBrief{}, fmt.Errorf("insert daily brief: %w", err)
	}
	stored.CreatedAt = stored.CreatedAt.UTC()
	stored.UpdatedAt = stored.UpdatedAt.UTC()

	for sectionPosition, section := range brief.sections {
		var sectionID string
		if err := transaction.QueryRow(
			ctx,
			insertDailyBriefSectionSQL,
			stored.ID,
			sectionPosition,
			section.heading,
			section.content,
			actor,
		).Scan(&sectionID); err != nil {
			return storedDailyBrief{}, fmt.Errorf("insert daily brief section %d: %w", sectionPosition, err)
		}

		for citationPosition, citation := range section.citations {
			if err := insertDailyBriefCitation(
				ctx,
				transaction,
				sectionID,
				citationPosition,
				citation,
				actor,
			); err != nil {
				return storedDailyBrief{}, fmt.Errorf(
					"insert daily brief section %d citation %d: %w",
					sectionPosition,
					citationPosition,
					err,
				)
			}
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return storedDailyBrief{}, fmt.Errorf("commit daily brief persistence: %w", err)
	}
	return stored, nil
}

func insertDailyBriefCitation(
	ctx context.Context,
	transaction pgx.Tx,
	sectionID string,
	position int,
	citation dailyBriefCitation,
	actor string,
) error {
	query := insertDailyBriefSourceRecordCitationSQL
	if citation.kind == dailyBriefCitationUpcomingEvent {
		query = insertDailyBriefUpcomingEventCitationSQL
	}

	var citationID string
	if err := transaction.QueryRow(
		ctx,
		query,
		sectionID,
		position,
		citation.id,
		citation.source,
		citation.url,
		actor,
	).Scan(&citationID); err != nil {
		return err
	}
	return nil
}

func validateDailyBriefPersistence(brief dailyBrief, actor string) error {
	if brief.region != calendar.RegionUnitedStates && brief.region != calendar.RegionEurozone {
		return fmt.Errorf("unsupported region %q", brief.region)
	}
	for _, window := range []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "publication", start: brief.publicationWindowStart, end: brief.publicationWindowEnd},
		{name: "event", start: brief.eventWindowStart, end: brief.eventWindowEnd},
	} {
		if window.start.IsZero() {
			return fmt.Errorf("%s window start is required", window.name)
		}
		if window.end.IsZero() {
			return fmt.Errorf("%s window end is required", window.name)
		}
		if window.end.Before(window.start) {
			return fmt.Errorf("%s window end must not be before window start", window.name)
		}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "provider", value: brief.provider},
		{name: "model", value: brief.model},
		{name: "actor", value: actor},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if len(brief.sections) == 0 {
		return errors.New("at least one section is required")
	}
	for sectionIndex, section := range brief.sections {
		if strings.TrimSpace(section.heading) == "" {
			return fmt.Errorf("section %d heading is required", sectionIndex)
		}
		if strings.TrimSpace(section.content) == "" {
			return fmt.Errorf("section %d content is required", sectionIndex)
		}
		if len(section.citations) == 0 {
			return fmt.Errorf("section %d must have at least one citation", sectionIndex)
		}
		for citationIndex, citation := range section.citations {
			if citation.kind != dailyBriefCitationSourceRecord && citation.kind != dailyBriefCitationUpcomingEvent {
				return fmt.Errorf("section %d citation %d has unsupported kind %q", sectionIndex, citationIndex, citation.kind)
			}
			if !validUUID(citation.id) {
				return fmt.Errorf("section %d citation %d ID must be a UUID", sectionIndex, citationIndex)
			}
			if strings.TrimSpace(citation.source) == "" {
				return fmt.Errorf("section %d citation %d source is required", sectionIndex, citationIndex)
			}
			if !validHTTPURL(citation.url) {
				return fmt.Errorf("section %d citation %d URL must be an absolute HTTP(S) URL", sectionIndex, citationIndex)
			}
		}
	}
	return nil
}

func validateStoredDailyBriefsQuery(
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) error {
	if region != calendar.RegionUnitedStates && region != calendar.RegionEurozone {
		return fmt.Errorf("unsupported region %q", region)
	}
	if windowStart.IsZero() {
		return errors.New("window start is required")
	}
	if windowEnd.IsZero() {
		return errors.New("window end is required")
	}
	if windowEnd.Before(windowStart) {
		return errors.New("window end must not be before window start")
	}
	if limit < 1 || limit > maxStoredDailyBriefsLimit {
		return fmt.Errorf("limit must be between 1 and %d", maxStoredDailyBriefsLimit)
	}
	return nil
}

func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil && id.Valid
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}

const insertDailyBriefSQL = `
INSERT INTO daily_briefs (
    region,
    publication_window_start,
    publication_window_end,
    event_window_start,
    event_window_end,
    provider,
    model,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
RETURNING id::text, created_at, updated_at, created_by, updated_by`

const insertDailyBriefSectionSQL = `
INSERT INTO daily_brief_sections (
    daily_brief_id,
    position,
    heading,
    content,
    created_by,
    updated_by
)
VALUES ($1, $2, $3, $4, $5, $5)
RETURNING id::text`

const insertDailyBriefSourceRecordCitationSQL = `
INSERT INTO daily_brief_citations (
    daily_brief_section_id,
    position,
    citation_kind,
    source_record_id,
    economic_event_id,
    source,
    source_url,
    created_by,
    updated_by
)
SELECT $1, $2, 'source_record', source_record.id, NULL, source_record.source, source_record.original_url, $6, $6
FROM source_records AS source_record
WHERE source_record.id = $3
  AND source_record.source = $4
  AND source_record.original_url = $5
RETURNING id::text`

const insertDailyBriefUpcomingEventCitationSQL = `
INSERT INTO daily_brief_citations (
    daily_brief_section_id,
    position,
    citation_kind,
    source_record_id,
    economic_event_id,
    source,
    source_url,
    created_by,
    updated_by
)
SELECT $1, $2, 'upcoming_event', NULL, economic_event.id, economic_event.source, economic_event.source_url, $6, $6
FROM economic_events AS economic_event
WHERE economic_event.id = $3
  AND economic_event.source = $4
  AND economic_event.source_url = $5
RETURNING id::text`
