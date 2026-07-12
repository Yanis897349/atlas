package searchcmd

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Yanis897349/atlas/internal/ingestion"
)

const indexSourceRecordsUsage = "usage: atlas index-source-records --from <RFC3339> --to <RFC3339> --limit <1-100> --actor <value>"

type indexSourceRecordsCommand struct {
	windowStart time.Time
	windowEnd   time.Time
	limit       int
	actor       string
}

func parseIndexSourceRecordsCommand(arguments []string) (indexSourceRecordsCommand, error) {
	flags := flag.NewFlagSet("index-source-records", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var from, to, limitValue, actor singleString
	flags.Var(&from, "from", "inclusive publication window start")
	flags.Var(&to, "to", "inclusive publication window end")
	flags.Var(&limitValue, "limit", "maximum source record count")
	flags.Var(&actor, "actor", "audit actor")
	if err := flags.Parse(arguments); err != nil {
		return indexSourceRecordsCommand{}, invalidArguments(err)
	}
	if flags.NArg() != 0 {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("unexpected positional arguments"))
	}

	for _, required := range []struct {
		name  string
		value *singleString
	}{{"from", &from}, {"to", &to}, {"limit", &limitValue}, {"actor", &actor}} {
		if !required.value.provided {
			return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("--%s is required", required.name))
		}
	}

	windowStart, err := time.Parse(time.RFC3339, from.value)
	if err != nil {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("--from must be RFC3339: %w", err))
	}
	windowEnd, err := time.Parse(time.RFC3339, to.value)
	if err != nil {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("--to must be RFC3339: %w", err))
	}
	if windowEnd.Before(windowStart) {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("--to must not be before --from"))
	}

	limit, err := strconv.Atoi(limitValue.value)
	if err != nil || limit < 1 || limit > ingestion.MaxRecentSourceRecordsLimit {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf(
			"--limit must be between 1 and %d", ingestion.MaxRecentSourceRecordsLimit,
		))
	}
	actor.value = strings.TrimSpace(actor.value)
	if actor.value == "" {
		return indexSourceRecordsCommand{}, invalidArguments(fmt.Errorf("--actor must not be blank"))
	}

	return indexSourceRecordsCommand{
		windowStart: windowStart.UTC(),
		windowEnd:   windowEnd.UTC(),
		limit:       limit,
		actor:       actor.value,
	}, nil
}
