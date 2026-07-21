package main

import (
	"context"
	"testing"
)

// stubPinger is a local Pinger that always returns nil (healthy).
type stubPinger struct{}

func (stubPinger) Ping(_ context.Context) error { return nil }

func TestBuildServer_NoPanic(t *testing.T) {
	mux := buildServer(stubPinger{}, stubPinger{})
	if mux == nil {
		t.Fatal("buildServer returned nil mux")
	}
}
