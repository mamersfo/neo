package neo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBoltURI(t *testing.T) {
	if got := BoltURI("localhost", 7687); got != "neo4j://localhost:7687" {
		t.Errorf("BoltURI = %q", got)
	}
}

func TestWaitReadyFastFailure(t *testing.T) {
	// Closed port: must fail with ErrNotReady quickly, not hang.
	ctx := context.Background()
	start := time.Now()
	err := WaitReady(ctx, BoltURI("127.0.0.1", 1), "neo4j", "wrong", 3*time.Second)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("took too long: %v", time.Since(start))
	}
}

func TestWaitReadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WaitReady(ctx, BoltURI("127.0.0.1", 1), "neo4j", "x", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestCheckRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := Check(ctx, BoltURI("127.0.0.1", 1), "neo4j", "x"); err == nil {
		t.Error("expected error for closed port")
	}
}
