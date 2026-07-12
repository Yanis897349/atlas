package app

import (
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/app/commandoutput"
)

func encodeCommandJSON(stdout io.Writer, subject string, value any) error {
	return commandoutput.EncodeJSON(stdout, subject, value)
}

func formatOutputTime(value time.Time) string {
	return commandoutput.FormatTime(value)
}
