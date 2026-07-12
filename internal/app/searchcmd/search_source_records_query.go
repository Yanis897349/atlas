package searchcmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Yanis897349/atlas/internal/search"
)

const searchSourceRecordsUsage = "usage: atlas search-source-records --query <text> --limit <1-100>"

type searchSourceRecordsCommand struct {
	query string
	limit int
}

func parseSearchSourceRecordsCommand(arguments []string) (searchSourceRecordsCommand, error) {
	flags := flag.NewFlagSet("search-source-records", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query, limitValue singleString
	flags.Var(&query, "query", "exact semantic search query")
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
	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > search.MaxSimilarSourceRecordsLimit {
		return searchSourceRecordsCommand{}, invalidSearchSourceRecordsArguments(fmt.Errorf(
			"--limit must be between 1 and %d", search.MaxSimilarSourceRecordsLimit,
		))
	}
	return searchSourceRecordsCommand{query: query.value, limit: limit}, nil
}

func invalidSearchSourceRecordsArguments(err error) error {
	return fmt.Errorf("invalid search-source-records arguments: %w; %s", err, searchSourceRecordsUsage)
}
