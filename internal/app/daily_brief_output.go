package app

import (
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
)

type dailyBriefCitationOutput struct {
	Kind   dailybrief.CitationKind `json:"kind"`
	ID     string                  `json:"id"`
	Source string                  `json:"source"`
	URL    string                  `json:"url"`
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

func newDailyBriefOutput(stored dailybrief.StoredBrief) dailyBriefOutput {
	brief := stored.Brief
	output := dailyBriefOutput{
		ID:     stored.ID,
		Region: brief.Region,
		PublicationWindow: dailyBriefWindowOutput{
			From: formatDailyBriefOutputTime(brief.PublicationWindowStart),
			To:   formatDailyBriefOutputTime(brief.PublicationWindowEnd),
		},
		EventWindow: dailyBriefWindowOutput{
			From: formatDailyBriefOutputTime(brief.EventWindowStart),
			To:   formatDailyBriefOutputTime(brief.EventWindowEnd),
		},
		Provider:  brief.Provider,
		Model:     brief.Model,
		Sections:  make([]dailyBriefSectionOutput, 0, len(brief.Sections)),
		CreatedAt: formatDailyBriefOutputTime(stored.CreatedAt),
		UpdatedAt: formatDailyBriefOutputTime(stored.UpdatedAt),
		CreatedBy: stored.CreatedBy,
		UpdatedBy: stored.UpdatedBy,
	}
	for _, section := range brief.Sections {
		sectionOutput := dailyBriefSectionOutput{
			Heading:   section.Heading,
			Content:   section.Content,
			Citations: make([]dailyBriefCitationOutput, 0, len(section.Citations)),
		}
		for _, citation := range section.Citations {
			sectionOutput.Citations = append(sectionOutput.Citations, dailyBriefCitationOutput{
				Kind:   citation.Kind,
				ID:     citation.ID,
				Source: citation.Source,
				URL:    citation.URL,
			})
		}
		output.Sections = append(output.Sections, sectionOutput)
	}
	return output
}

func formatDailyBriefOutputTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
