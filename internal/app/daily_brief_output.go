package app

import (
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

type dailyBriefCitationOutput struct {
	Kind   dailyBriefCitationKind `json:"kind"`
	ID     string                 `json:"id"`
	Source string                 `json:"source"`
	URL    string                 `json:"url"`
}

type dailyBriefSectionOutput struct {
	Heading   string                     `json:"heading"`
	Content   string                     `json:"content"`
	Citations []dailyBriefCitationOutput `json:"citations"`
}

type dailyBriefOutput struct {
	ID                string                    `json:"id"`
	Region            calendar.Region           `json:"region"`
	PublicationWindow dailyBriefWindowOutput    `json:"publication_window"`
	EventWindow       dailyBriefWindowOutput    `json:"event_window"`
	Provider          string                    `json:"provider"`
	Model             string                    `json:"model"`
	Sections          []dailyBriefSectionOutput `json:"sections"`
	CreatedAt         string                    `json:"created_at"`
	UpdatedAt         string                    `json:"updated_at"`
	CreatedBy         string                    `json:"created_by"`
	UpdatedBy         string                    `json:"updated_by"`
}

func newDailyBriefOutput(stored storedDailyBrief) dailyBriefOutput {
	brief := stored.dailyBrief
	output := dailyBriefOutput{
		ID:     stored.ID,
		Region: brief.region,
		PublicationWindow: dailyBriefWindowOutput{
			From: formatDailyBriefOutputTime(brief.publicationWindowStart),
			To:   formatDailyBriefOutputTime(brief.publicationWindowEnd),
		},
		EventWindow: dailyBriefWindowOutput{
			From: formatDailyBriefOutputTime(brief.eventWindowStart),
			To:   formatDailyBriefOutputTime(brief.eventWindowEnd),
		},
		Provider:  brief.provider,
		Model:     brief.model,
		Sections:  make([]dailyBriefSectionOutput, 0, len(brief.sections)),
		CreatedAt: formatDailyBriefOutputTime(stored.CreatedAt),
		UpdatedAt: formatDailyBriefOutputTime(stored.UpdatedAt),
		CreatedBy: stored.CreatedBy,
		UpdatedBy: stored.UpdatedBy,
	}
	for _, section := range brief.sections {
		sectionOutput := dailyBriefSectionOutput{
			Heading:   section.heading,
			Content:   section.content,
			Citations: make([]dailyBriefCitationOutput, 0, len(section.citations)),
		}
		for _, citation := range section.citations {
			sectionOutput.Citations = append(sectionOutput.Citations, dailyBriefCitationOutput{
				Kind:   citation.kind,
				ID:     citation.id,
				Source: citation.source,
				URL:    citation.url,
			})
		}
		output.Sections = append(output.Sections, sectionOutput)
	}
	return output
}

func formatDailyBriefOutputTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
