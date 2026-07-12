package watchlistcmd

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type singleString struct {
	value    string
	provided bool
}

func (value *singleString) String() string {
	return value.value
}

func (value *singleString) Set(input string) error {
	if value.provided {
		return fmt.Errorf("must only be provided once")
	}
	value.value = input
	value.provided = true
	return nil
}

type repeatedStrings []string

func (values *repeatedStrings) String() string {
	return strings.Join(*values, ",")
}

func (values *repeatedStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func validWatchlistID(id string) bool {
	var parsed pgtype.UUID
	return parsed.Scan(id) == nil && parsed.Valid
}

func invalidWatchlistArguments(commandName, usage string, err error) error {
	return fmt.Errorf("invalid %s arguments: %w; %s", commandName, err, usage)
}
