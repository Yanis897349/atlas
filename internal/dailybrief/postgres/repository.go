package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/jackc/pgx/v5"
)

// MaxStoredBriefsLimit bounds one stored-daily-brief retrieval.
const MaxStoredBriefsLimit = 100

// DB is the PostgreSQL operation used by Repository.
type DB interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Repository persists generated daily briefs.
type Repository struct {
	db DB
}

// NewRepository returns a daily-brief repository backed by db.
func NewRepository(db DB) (*Repository, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL database is required")
	}
	return &Repository{db: db}, nil
}

// PersistDailyBrief atomically stores a generated daily brief.
func (repository *Repository) PersistDailyBrief(
	ctx context.Context,
	brief dailybrief.Brief,
	actor string,
) (dailybrief.StoredBrief, error) {
	actor = strings.TrimSpace(actor)
	brief.Provider = strings.TrimSpace(brief.Provider)
	brief.Model = strings.TrimSpace(brief.Model)
	if err := validateDailyBriefPersistence(brief, actor); err != nil {
		return dailybrief.StoredBrief{}, err
	}

	brief.PublicationWindowStart = brief.PublicationWindowStart.UTC()
	brief.PublicationWindowEnd = brief.PublicationWindowEnd.UTC()
	brief.EventWindowStart = brief.EventWindowStart.UTC()
	brief.EventWindowEnd = brief.EventWindowEnd.UTC()

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return dailybrief.StoredBrief{}, fmt.Errorf("begin daily brief persistence: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()

	stored := dailybrief.StoredBrief{Brief: brief}
	if err := transaction.QueryRow(
		ctx,
		insertDailyBriefSQL,
		brief.Region,
		brief.PublicationWindowStart,
		brief.PublicationWindowEnd,
		brief.EventWindowStart,
		brief.EventWindowEnd,
		brief.Provider,
		brief.Model,
		actor,
	).Scan(
		&stored.ID,
		&stored.CreatedAt,
		&stored.UpdatedAt,
		&stored.CreatedBy,
		&stored.UpdatedBy,
	); err != nil {
		return dailybrief.StoredBrief{}, fmt.Errorf("insert daily brief: %w", err)
	}
	stored.CreatedAt = stored.CreatedAt.UTC()
	stored.UpdatedAt = stored.UpdatedAt.UTC()

	for sectionPosition, section := range brief.Sections {
		var sectionID string
		if err := transaction.QueryRow(
			ctx,
			insertDailyBriefSectionSQL,
			stored.ID,
			sectionPosition,
			section.Heading,
			section.Content,
			actor,
		).Scan(&sectionID); err != nil {
			return dailybrief.StoredBrief{}, fmt.Errorf("insert daily brief section %d: %w", sectionPosition, err)
		}

		for citationPosition, citation := range section.Citations {
			if err := insertDailyBriefCitation(
				ctx,
				transaction,
				sectionID,
				citationPosition,
				citation,
				actor,
			); err != nil {
				return dailybrief.StoredBrief{}, fmt.Errorf(
					"insert daily brief section %d citation %d: %w",
					sectionPosition,
					citationPosition,
					err,
				)
			}
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return dailybrief.StoredBrief{}, fmt.Errorf("commit daily brief persistence: %w", err)
	}
	return stored, nil
}

func insertDailyBriefCitation(
	ctx context.Context,
	transaction pgx.Tx,
	sectionID string,
	position int,
	citation dailybrief.Citation,
	actor string,
) error {
	query := insertDailyBriefSourceRecordCitationSQL
	if citation.Kind == dailybrief.CitationUpcomingEvent {
		query = insertDailyBriefUpcomingEventCitationSQL
	}

	var citationID string
	if err := transaction.QueryRow(
		ctx,
		query,
		sectionID,
		position,
		citation.ID,
		citation.Source,
		citation.URL,
		actor,
	).Scan(&citationID); err != nil {
		return err
	}
	return nil
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
