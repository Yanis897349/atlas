package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

func (repository *dailyBriefRepository) StoredDailyBriefs(
	ctx context.Context,
	region calendar.Region,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]storedDailyBrief, error) {
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

	briefs := make([]storedDailyBrief, 0, limit)
	var currentBriefID, currentSectionID string
	for rows.Next() {
		var (
			briefID, sectionID string
			brief              dailyBrief
			createdAt          time.Time
			updatedAt          time.Time
			createdBy          string
			updatedBy          string
			section            dailyBriefSection
			citation           dailyBriefCitation
		)
		if err := rows.Scan(
			&briefID,
			&brief.region,
			&brief.publicationWindowStart,
			&brief.publicationWindowEnd,
			&brief.eventWindowStart,
			&brief.eventWindowEnd,
			&brief.provider,
			&brief.model,
			&createdAt,
			&updatedAt,
			&createdBy,
			&updatedBy,
			&sectionID,
			&section.heading,
			&section.content,
			&citation.kind,
			&citation.id,
			&citation.source,
			&citation.url,
		); err != nil {
			return nil, fmt.Errorf("scan stored daily brief: %w", err)
		}
		brief.publicationWindowStart = brief.publicationWindowStart.UTC()
		brief.publicationWindowEnd = brief.publicationWindowEnd.UTC()
		brief.eventWindowStart = brief.eventWindowStart.UTC()
		brief.eventWindowEnd = brief.eventWindowEnd.UTC()
		createdAt = createdAt.UTC()
		updatedAt = updatedAt.UTC()

		if briefID != currentBriefID {
			brief.sections = make([]dailyBriefSection, 0, 1)
			briefs = append(briefs, storedDailyBrief{
				ID:         briefID,
				dailyBrief: brief,
				CreatedAt:  createdAt,
				UpdatedAt:  updatedAt,
				CreatedBy:  createdBy,
				UpdatedBy:  updatedBy,
			})
			currentBriefID = briefID
			currentSectionID = ""
		}

		briefIndex := len(briefs) - 1
		if sectionID != currentSectionID {
			section.citations = make([]dailyBriefCitation, 0, 1)
			briefs[briefIndex].sections = append(briefs[briefIndex].sections, section)
			currentSectionID = sectionID
		}
		sectionIndex := len(briefs[briefIndex].sections) - 1
		briefs[briefIndex].sections[sectionIndex].citations = append(
			briefs[briefIndex].sections[sectionIndex].citations,
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
