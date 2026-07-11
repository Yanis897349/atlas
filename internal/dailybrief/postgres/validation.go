package postgres

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
	"github.com/Yanis897349/atlas/internal/dailybrief"
	"github.com/jackc/pgx/v5/pgtype"
)

func validateDailyBriefPersistence(brief dailybrief.Brief, actor string) error {
	if brief.Region != calendar.RegionUnitedStates && brief.Region != calendar.RegionEurozone {
		return fmt.Errorf("unsupported region %q", brief.Region)
	}
	for _, window := range []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{name: "publication", start: brief.PublicationWindowStart, end: brief.PublicationWindowEnd},
		{name: "event", start: brief.EventWindowStart, end: brief.EventWindowEnd},
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
		{name: "provider", value: brief.Provider},
		{name: "model", value: brief.Model},
		{name: "actor", value: actor},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if len(brief.Sections) == 0 {
		return errors.New("at least one section is required")
	}
	for sectionIndex, section := range brief.Sections {
		if strings.TrimSpace(section.Heading) == "" {
			return fmt.Errorf("section %d heading is required", sectionIndex)
		}
		if strings.TrimSpace(section.Content) == "" {
			return fmt.Errorf("section %d content is required", sectionIndex)
		}
		if len(section.Citations) == 0 {
			return fmt.Errorf("section %d must have at least one citation", sectionIndex)
		}
		for citationIndex, citation := range section.Citations {
			if citation.Kind != dailybrief.CitationSourceRecord && citation.Kind != dailybrief.CitationUpcomingEvent {
				return fmt.Errorf("section %d citation %d has unsupported kind %q", sectionIndex, citationIndex, citation.Kind)
			}
			if !validUUID(citation.ID) {
				return fmt.Errorf("section %d citation %d ID must be a UUID", sectionIndex, citationIndex)
			}
			if strings.TrimSpace(citation.Source) == "" {
				return fmt.Errorf("section %d citation %d source is required", sectionIndex, citationIndex)
			}
			if !validHTTPURL(citation.URL) {
				return fmt.Errorf("section %d citation %d URL must be an absolute HTTP(S) URL", sectionIndex, citationIndex)
			}
		}
	}
	return nil
}

func validateStoredDailyBriefsQuery(region calendar.Region, windowStart, windowEnd time.Time, limit int) error {
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
	if limit < 1 || limit > MaxStoredBriefsLimit {
		return fmt.Errorf("limit must be between 1 and %d", MaxStoredBriefsLimit)
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
