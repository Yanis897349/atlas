// Package output provides deterministic Atlas command output formatting.
package output

import (
	"bytes"
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

// EncodeJSONBuffered encodes one JSON result before writing it to stdout.
func EncodeJSONBuffered(stdout io.Writer, subject string, value any) error {
	var encoded bytes.Buffer
	if err := EncodeJSON(&encoded, subject, value); err != nil {
		return err
	}
	written, err := stdout.Write(encoded.Bytes())
	if err != nil {
		return fmt.Errorf("write %s: %w", subject, err)
	}
	if written != encoded.Len() {
		return fmt.Errorf("write %s: %w", subject, io.ErrShortWrite)
	}
	return nil
}

// FormatTime returns one timestamp in canonical UTC command-output form.
func FormatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
