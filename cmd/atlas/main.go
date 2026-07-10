package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Yanis897349/atlas/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args[1:], app.Dependencies{Stdout: os.Stdout}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "atlas: %v\n", err)
		os.Exit(1)
	}
}
