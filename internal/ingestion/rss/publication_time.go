package rss

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func parsePublicationTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("publication time is required")
	}

	normalizedValue := value
	fields := strings.Fields(value)
	if len(fields) > 0 {
		zone := fields[len(fields)-1]
		if offset, ok := rfc822ZoneOffset(zone); ok {
			normalizedValue = strings.TrimSpace(strings.TrimSuffix(value, zone)) +
				fmt.Sprintf(" %+03d00", offset/(60*60))
		}
	}

	formats := []string{
		time.RFC1123Z,
		time.RFC822Z,
		time.RFC3339,
	}
	for _, format := range formats {
		publishedAt, err := time.Parse(format, normalizedValue)
		if err == nil {
			return publishedAt.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid publication time %q", value)
}

func rfc822ZoneOffset(zone string) (int, bool) {
	const secondsPerHour = 60 * 60

	switch zone {
	case "UT", "UTC", "GMT", "Z":
		return 0, true
	case "EST":
		return -5 * secondsPerHour, true
	case "EDT":
		return -4 * secondsPerHour, true
	case "CST":
		return -6 * secondsPerHour, true
	case "CDT":
		return -5 * secondsPerHour, true
	case "MST":
		return -7 * secondsPerHour, true
	case "MDT":
		return -6 * secondsPerHour, true
	case "PST":
		return -8 * secondsPerHour, true
	case "PDT":
		return -7 * secondsPerHour, true
	}

	if len(zone) != 1 {
		return 0, false
	}

	switch letter := zone[0]; {
	case letter >= 'A' && letter <= 'I':
		return int(letter-'A'+1) * secondsPerHour, true
	case letter >= 'K' && letter <= 'M':
		return int(letter-'A') * secondsPerHour, true
	case letter >= 'N' && letter <= 'Y':
		return -int(letter-'N'+1) * secondsPerHour, true
	default:
		return 0, false
	}
}
