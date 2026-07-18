package postgres

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/intelligence"
	atlasuuid "github.com/Yanis897349/atlas/internal/uuid"
)

func normalizeAndValidateObservation(
	observation intelligence.Observation,
	actor string,
) (intelligence.Observation, string, error) {
	eventID, valid := atlasuuid.Normalize(observation.EconomicEventID)
	if !valid {
		return intelligence.Observation{}, "", errors.New("economic event ID must be a UUID")
	}
	observation.EconomicEventID = eventID
	observation.Source = strings.TrimSpace(observation.Source)
	if observation.Source == "" {
		return intelligence.Observation{}, "", errors.New("source is required")
	}
	observation.SourceObservationID = strings.TrimSpace(observation.SourceObservationID)
	if observation.SourceObservationID == "" {
		return intelligence.Observation{}, "", errors.New("source observation ID is required")
	}
	parsedURL, err := url.Parse(observation.SourceURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return intelligence.Observation{}, "", errors.New("source URL must be an absolute HTTP(S) URL")
	}
	if observation.ObservedAt.IsZero() {
		return intelligence.Observation{}, "", errors.New("observation time is required")
	}
	observation.ObservedAt = observation.ObservedAt.UTC().Truncate(time.Microsecond)

	for _, value := range []struct {
		name  string
		field **string
	}{
		{name: "consensus", field: &observation.Consensus},
		{name: "previous", field: &observation.Previous},
		{name: "actual", field: &observation.Actual},
	} {
		if *value.field == nil {
			continue
		}
		normalized := strings.TrimSpace(**value.field)
		if normalized == "" {
			return intelligence.Observation{}, "", fmt.Errorf("%s value must not be blank", value.name)
		}
		*value.field = &normalized
	}
	if observation.Consensus == nil && observation.Previous == nil && observation.Actual == nil {
		return intelligence.Observation{}, "", errors.New("at least one observation value is required")
	}

	actor = strings.TrimSpace(actor)
	if actor == "" {
		return intelligence.Observation{}, "", errors.New("actor is required")
	}
	return observation, actor, nil
}

func normalizeAndValidateEventObservationsQuery(eventID string, limit int) (string, error) {
	normalized, valid := atlasuuid.Normalize(eventID)
	if !valid {
		return "", errors.New("economic event ID must be a UUID")
	}
	if limit < 1 || limit > intelligence.MaxEventObservationsLimit {
		return "", fmt.Errorf("limit must be between 1 and %d", intelligence.MaxEventObservationsLimit)
	}
	return normalized, nil
}

func normalizeAndValidateObservationRevisionsQuery(
	eventID string,
	source string,
	sourceObservationID string,
	limit int,
) (string, string, string, error) {
	normalizedEventID, valid := atlasuuid.Normalize(eventID)
	if !valid {
		return "", "", "", errors.New("economic event ID must be a UUID")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", "", errors.New("source is required")
	}
	sourceObservationID = strings.TrimSpace(sourceObservationID)
	if sourceObservationID == "" {
		return "", "", "", errors.New("source observation ID is required")
	}
	if limit < 1 || limit > intelligence.MaxEventObservationsLimit {
		return "", "", "", fmt.Errorf(
			"limit must be between 1 and %d",
			intelligence.MaxEventObservationsLimit,
		)
	}
	return normalizedEventID, source, sourceObservationID, nil
}
