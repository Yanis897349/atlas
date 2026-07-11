package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
)

// StoredDailyBriefs returns briefs created within the inclusive time window.
func (repository *Repository) StoredDailyBriefs(
	ctx context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]dailybrief.StoredBrief, error) {
	if err := validateStoredDailyBriefsQuery(region, windowStart, windowEnd, limit); err != nil {
		return nil, err
	}

	rows, err := repository.db.Query(
		ctx,
		storedDailyBriefsSQL,
		region,
		windowStart.UTC(),
		windowEnd.UTC(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query stored daily briefs: %w", err)
	}
	defer rows.Close()

	briefs := make([]dailybrief.StoredBrief, 0, limit)
	var currentBriefID, currentSectionID string
	for rows.Next() {
		var (
			briefID, sectionID string
			brief              dailybrief.Brief
			createdAt          time.Time
			updatedAt          time.Time
			createdBy          string
			updatedBy          string
			section            dailybrief.Section
			citation           dailybrief.Citation
		)
		if err := rows.Scan(
			&briefID,
			&brief.Region,
			&brief.PublicationWindowStart,
			&brief.PublicationWindowEnd,
			&brief.EventWindowStart,
			&brief.EventWindowEnd,
			&brief.Provider,
			&brief.Model,
			&createdAt,
			&updatedAt,
			&createdBy,
			&updatedBy,
			&sectionID,
			&section.Heading,
			&section.Content,
			&citation.Kind,
			&citation.ID,
			&citation.Source,
			&citation.URL,
		); err != nil {
			return nil, fmt.Errorf("scan stored daily brief: %w", err)
		}
		brief.PublicationWindowStart = brief.PublicationWindowStart.UTC()
		brief.PublicationWindowEnd = brief.PublicationWindowEnd.UTC()
		brief.EventWindowStart = brief.EventWindowStart.UTC()
		brief.EventWindowEnd = brief.EventWindowEnd.UTC()
		createdAt = createdAt.UTC()
		updatedAt = updatedAt.UTC()

		if briefID != currentBriefID {
			brief.Sections = make([]dailybrief.Section, 0, 1)
			briefs = append(briefs, dailybrief.StoredBrief{
				ID:        briefID,
				Brief:     brief,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				CreatedBy: createdBy,
				UpdatedBy: updatedBy,
			})
			currentBriefID = briefID
			currentSectionID = ""
		}

		briefIndex := len(briefs) - 1
		if sectionID != currentSectionID {
			section.Citations = make([]dailybrief.Citation, 0, 1)
			briefs[briefIndex].Sections = append(briefs[briefIndex].Sections, section)
			currentSectionID = sectionID
		}
		sectionIndex := len(briefs[briefIndex].Sections) - 1
		briefs[briefIndex].Sections[sectionIndex].Citations = append(
			briefs[briefIndex].Sections[sectionIndex].Citations,
			citation,
		)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored daily briefs: %w", err)
	}
	return briefs, nil
}

const storedDailyBriefsSQL = `
WITH selected_briefs AS (
    SELECT
        id,
        region,
        publication_window_start,
        publication_window_end,
        event_window_start,
        event_window_end,
        provider,
        model,
        created_at,
        updated_at,
        created_by,
        updated_by
    FROM daily_briefs
    WHERE region = $1
      AND created_at >= $2
      AND created_at <= $3
    ORDER BY created_at DESC, id ASC
    LIMIT $4
)
SELECT
    brief.id::text,
    brief.region,
    brief.publication_window_start,
    brief.publication_window_end,
    brief.event_window_start,
    brief.event_window_end,
    brief.provider,
    brief.model,
    brief.created_at,
    brief.updated_at,
    brief.created_by,
    brief.updated_by,
    section.id::text,
    section.heading,
    section.content,
    citation.citation_kind,
    COALESCE(citation.source_record_id, citation.economic_event_id)::text,
    citation.source,
    citation.source_url
FROM selected_briefs AS brief
JOIN daily_brief_sections AS section ON section.daily_brief_id = brief.id
JOIN daily_brief_citations AS citation ON citation.daily_brief_section_id = section.id
ORDER BY brief.created_at DESC, brief.id ASC, section.position ASC, citation.position ASC`
