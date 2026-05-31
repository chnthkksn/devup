package cleanup

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func WithSignals(ctx context.Context, onSignal func()) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		if onSignal != nil {
			onSignal()
		}
		cancel()
		// Restore default signal handling so a second Ctrl+C force-exits.
		signal.Reset(os.Interrupt, syscall.SIGTERM)
	}()
	return cctx, cancel
}

