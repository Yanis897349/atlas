package searchcmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Yanis897349/atlas/internal/search"
)

const searchSourceRecordsUsage = "usage: atlas search-source-records --query <text> [--source <source>] --limit <1-100>"

type searchSourceRecordsCommand struct {
	query  string
	source *string
	limit  int
}

func parseSearchSourceRecordsCommand(arguments []string) (searchSourceRecordsCommand, error) {
	flags := flag.NewFlagSet("search-source-records", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query, source, limitValue singleString
	flags.Var(&query, "query", "exact semantic search query")
	flags.Var(&source, "source", "exact canonical source filter")
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
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf(
			"--limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit,
		))
	}
	return searchSourceRecordsCommand{query: query.value, source: sourceFilter, limit: limit}, nil
}

func invalidSearchSourceRecordsArguments(err error) error {
	return fmt.Errorf("invalid search-source-records arguments: %w; %s", err, searchSourceRecordsUsage)
}
