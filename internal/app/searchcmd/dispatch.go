// Package searchcmd parses and executes Atlas search commands.
package searchcmd

import (
	"context"
	"io"

	"github.com/Yanis897349/atlas/internal/search"
)

// Command is one validated search command.
type Command struct {
	name   string
	index  indexSourceRecordsCommand
	search searchSourceRecordsCommand
}

// Parse recognizes and validates one search command.
func Parse(arguments []string) (Command, bool, error) {
	if len(arguments) == 0 {
		return Command{}, false, nil
	}
	switch arguments[0] {
	case "index-source-records":
		index, err := parseIndexSourceRecordsCommand(arguments[1:])
		if err != nil {
			return Command{}, true, err
		}
		return Command{name: arguments[0], index: index}, true, nil
	case "search-source-records":
		searchCommand, err := parseSearchSourceRecordsCommand(arguments[1:])
		if err != nil {
			return Command{}, true, err
		}
		return Command{name: arguments[0], search: searchCommand}, true, nil
	default:
		return Command{}, false, nil
	}
}

// Run executes one validated search command.
func Run(
	ctx context.Context,
	reader search.SourceRecordReader,
	embedder search.Embedder,
	writer search.SourceRecordEmbeddingWriter,
	similarReader search.SimilarSourceRecordReader,
	stdout io.Writer,
	command Command,
) error {
	switch command.name {
	case "index-source-records":
		return runIndexSourceRecords(ctx, reader, embedder, writer, stdout, command.index)
	case "search-source-records":
		return runSearchSourceRecords(ctx, embedder, similarReader, stdout, command.search)
	default:
		panic("validated search command is not handled")
	}
}
