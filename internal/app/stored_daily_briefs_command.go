package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Yanis897349/atlas/internal/calendar"
)

type storedDailyBriefsQuery struct {
	region      calendar.Region
	windowStart time.Time
	windowEnd   time.Time
	limit       int
}

type storedDailyBriefsReader interface {
	StoredDailyBriefs(context.Context, calendar.Region, time.Time, time.Time, int) ([]storedDailyBrief, error)
}

func runStoredDailyBriefs(
	ctx context.Context,
	repository storedDailyBriefsReader,
	stdout io.Writer,
	query storedDailyBriefsQuery,
) error {
	briefs, err := repository.StoredDailyBriefs(
		ctx,
		query.region,
		query.windowStart,
		query.windowEnd,
		query.limit,
	)
	if err != nil {
		return fmt.Errorf("retrieve stored daily briefs: %w", err)
	}

	output := make([]dailyBriefOutput, 0, len(briefs))
	for _, brief := range briefs {
		output = append(output, newDailyBriefOutput(brief))
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode stored daily briefs: %w", err)
	}
	return nil
}
