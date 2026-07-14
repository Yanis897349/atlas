package searchcmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/search"
)

const searchSourceRecordsUsage = "usage: atlas search-source-records --query <text> [--source <source>] [--from <RFC3339> --to <RFC3339>] --limit <1-100>"

type searchSourceRecordsCommand struct {
	query   string
	filters search.SimilarSourceRecordFilters
	limit   int
}

func parseSearchSourceRecordsCommand(arguments []string) (searchSourceRecordsCommand, error) {
	flags := flag.NewFlagSet("search-source-records", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query, source, from, to, limitValue singleString
	flags.Var(&query, "query", "exact semantic search query")
	flags.Var(&source, "source", "exact canonical source filter")
	flags.Var(&from, "from", "inclusive publication window start")
	flags.Var(&to, "to", "inclusive publication window end")
	flags.Var(&limitValue, "limit", "maximum result count")
	if err := flags.Parse(arguments); err != nil {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(err)
	}
	if flags.NArg() != 0 {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("unexpected positional arguments"))
	}
	if !query.provided {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--query is required"))
	}
	if !limitValue.provided {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--limit is required"))
	}
	if strings.TrimSpace(query.value) == "" {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--query must not be blank"))
	}
	var sourceFilter *string
	if source.provided {
		normalizedSource := strings.TrimSpace(source.value)
		if normalizedSource == "" {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--source must not be blank"))
		}
		sourceFilter = &normalizedSource
	}
	if from.provided != to.provided {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf(
			"--from and --to must be supplied together",
		))
	}
	var windowStart, windowEnd *time.Time
	if from.provided {
		parsedStart, err := time.Parse(time.RFC3339, from.value)
		if err != nil {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--from must be RFC3339: %w", err))
		}
		parsedEnd, err := time.Parse(time.RFC3339, to.value)
		if err != nil {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--to must be RFC3339: %w", err))
		}
		if parsedStart.IsZero() {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--from must not be zero"))
		}
		if parsedEnd.IsZero() {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--to must not be zero"))
		}
		if parsedEnd.Before(parsedStart) {
			return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf("--to must not be before --from"))
		}
		parsedStart = parsedStart.UTC()
		parsedEnd = parsedEnd.UTC()
		windowStart = &parsedStart
		windowEnd = &parsedEnd
	}
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf(
			"--limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit,
		))
	}
	return searchSourceRecordsCommand{
		query: query.value,
		filters: search.SimilarSourceRecordFilters{
			Source:                 sourceFilter,
			PublicationWindowStart: windowStart,
			PublicationWindowEnd:   windowEnd,
		},
		limit: limit,
	}, nil
}

func invalidSearchSourceRecordsArguments(err error) error {
	return fmt.Errorf("invalid search-source-records arguments: %w; %s", err, searchSourceRecordsUsage)
}
