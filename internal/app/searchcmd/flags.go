package searchcmd

import "fmt"

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

func invalidIndexSourceRecordsArguments(err error) error {
	return fmt.Errorf("invalid index-source-records arguments: %w; %s", err, indexSourceRecordsUsage)
}
