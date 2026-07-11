package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/dailybrief"
)

type storedDailyBriefsQuery = regionWindowQuery

func runStoredDailyBriefs(
	ctx context.Context,
	repository dailybrief.Reader,
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

	return encodeCommandJSON(stdout, "stored daily briefs", output)
}
