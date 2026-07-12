package app

import (
	"context"
	"io"

	"github.com/Yanis897349/atlas/internal/watchlist"
)

type watchlistCommand struct {
	name        string
	create      createWatchlistCommand
	update      updateWatchlistCommand
	delete      deleteWatchlistCommand
	lookup      watchlistQuery
	list        watchlistsQuery
	linkEvent   linkWatchlistEventCommand
	unlinkEvent unlinkWatchlistEventCommand
	listEvents  watchlistEventsQuery
}

func parseWatchlistCommand(arguments []string) (watchlistCommand, bool, error) {
	name := arguments[0]
	command := watchlistCommand{name: name}
	var err error

	switch name {
	case "create-watchlist":
		command.create, err = parseCreateWatchlistCommand(arguments[1:])
	case "update-watchlist":
		command.update, err = parseUpdateWatchlistCommand(arguments[1:])
	case "delete-watchlist":
		command.delete, err = parseDeleteWatchlistCommand(arguments[1:])
	case "watchlist":
		command.lookup, err = parseWatchlistQuery(arguments[1:])
	case "watchlists":
		command.list, err = parseWatchlistsQuery(arguments[1:])
	case "link-watchlist-event":
		command.linkEvent, err = parseLinkWatchlistEventCommand(arguments[1:])
	case "unlink-watchlist-event":
		command.unlinkEvent, err = parseUnlinkWatchlistEventCommand(arguments[1:])
	case "watchlist-events":
		command.listEvents, err = parseWatchlistEventsQuery(arguments[1:])
	default:
		return watchlistCommand{}, false, nil
	}
	if err != nil {
		return watchlistCommand{}, true, err
	}
	return command, true, nil
}

func runWatchlistCommand(
	ctx context.Context,
	repository watchlistCommandRepository,
	stdout io.Writer,
	command watchlistCommand,
) error {
	switch command.name {
	case "create-watchlist":
		return runCreateWatchlist(ctx, repository, stdout, command.create)
	case "update-watchlist":
		return runUpdateWatchlist(ctx, repository, stdout, command.update)
	case "delete-watchlist":
		return runDeleteWatchlist(ctx, repository, command.delete)
	case "watchlist":
		return runWatchlist(ctx, repository, stdout, command.lookup)
	case "watchlists":
		return runWatchlists(ctx, repository, stdout, command.list)
	case "link-watchlist-event":
		return runLinkWatchlistEvent(ctx, repository, stdout, command.linkEvent)
	case "unlink-watchlist-event":
		return runUnlinkWatchlistEvent(ctx, repository, command.unlinkEvent)
	case "watchlist-events":
		return runWatchlistEvents(ctx, repository, stdout, command.listEvents)
	default:
		panic("validated watchlist command is not handled")
	}
}

type watchlistCreator interface {
	CreateWatchlist(context.Context, watchlist.Definition, string) (watchlist.StoredWatchlist, error)
}

type watchlistUpdater interface {
	UpdateWatchlist(context.Context, string, watchlist.Definition, string) (watchlist.StoredWatchlist, error)
}

type watchlistDeleter interface {
	DeleteWatchlist(context.Context, string) error
}

type watchlistLookup interface {
	Watchlist(context.Context, string) (watchlist.StoredWatchlist, error)
}

type watchlistLister interface {
	Watchlists(context.Context, int) ([]watchlist.StoredWatchlist, error)
}

type watchlistEventLinkCreator interface {
	CreateEventLink(context.Context, string, string, string, string) (watchlist.StoredEventLink, error)
}

type watchlistEventLinkDeleter interface {
	DeleteEventLink(context.Context, string, string, string) error
}

type watchlistEventLinkReader interface {
	EventLinks(context.Context, string, string, int) ([]watchlist.StoredEventLink, error)
}

type watchlistCommandRepository interface {
	watchlistCreator
	watchlistUpdater
	watchlistDeleter
	watchlistLookup
	watchlistLister
	watchlistEventLinkCreator
	watchlistEventLinkDeleter
	watchlistEventLinkReader
}
