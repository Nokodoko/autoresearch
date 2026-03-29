package guardrails

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandler registers SIGINT/SIGTERM handlers and calls guard.RequestShutdown.
// Returns a context that is cancelled on signal.
func SetupSignalHandler(ctx context.Context, guard *Guard) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			guard.RequestShutdown()
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx
}
