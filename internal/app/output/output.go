// Package output provides deterministic Atlas command output formatting.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// EncodeJSON writes one JSON command result without HTML escaping.
func EncodeJSON(stdout io.Writer, subject string, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", subject, err)
	}
	return nil
}

// FormatTime returns one timestamp in canonical UTC command-output form.
func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
