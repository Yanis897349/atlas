package app

import (
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/app/output"
)

func encodeCommandJSON(stdout io.Writer, subject string, value any) error {
	return output.EncodeJSON(stdout, subject, value)
}

func formatOutputTime(value time.Time) string {
	return output.FormatTime(value)
}
