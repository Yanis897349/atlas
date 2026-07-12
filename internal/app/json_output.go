package app

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

func encodeCommandJSON(stdout io.Writer, subject string, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", subject, err)
	}
	return nil
}

func formatOutputTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
