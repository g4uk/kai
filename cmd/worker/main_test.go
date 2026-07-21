package main

import (
	"context"
	"testing"
	"time"
)

func TestRunLoop_Cancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before runLoop is even called

	done := make(chan struct{})
	go func() {
		runLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// runLoop returned promptly after ctx was cancelled — expected
	case <-time.After(1 * time.Second):
		t.Fatal("runLoop did not return within 1s after context cancellation")
	}
}
