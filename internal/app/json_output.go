package app

import (
	"encoding/json"
	"fmt"
	"io"
)

func encodeCommandJSON(stdout io.Writer, subject string, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %s: %w", subject, err)
	}
	return nil
}
