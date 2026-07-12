package watchlistcmd

import (
	"context"
	"fmt"
	"io"

	"github.com/Yanis897349/atlas/internal/app/output"
	"github.com/Yanis897349/atlas/internal/watchlist"
)

func runLinkWatchlistEvents(
	ctx context.Context,
	candidates watchlist.EventCandidateReader,
	reader watchlist.WatchlistReader,
	writer watchlist.EventLinkWriter,
	stdout io.Writer,
	command linkWatchlistEventsCommand,
) error {
	links, err := watchlist.LinkRelevantEventCandidates(
		ctx,
		candidates,
		reader,
		writer,
		command.watchlistID,
		command.windowStart,
		command.windowEnd,
		command.limit,
		command.actor,
	)
	if err != nil {
		return fmt.Errorf("link watchlist event candidates: %w", err)
	}

	result := make([]watchlistEventOutput, 0, len(links))
	for _, link := range links {
		result = append(result, newWatchlistEventOutput(link))
	}
	return output.EncodeJSON(stdout, "linked watchlist event candidates", result)
}
