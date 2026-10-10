//go:build integration

// Integration cycle: pull → start → inspect → stop → remove against a real
// Docker daemon + Neo4j image. Run with:
//
//	go test -tags integration ./internal/docker -run TestStartStopCycle -v
//
// Skipped by default (needs Docker, pulls ~400MB on first run).
package docker

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type logWriter struct{ t *testing.T }

func (w logWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

func testWriter(t *testing.T) io.Writer { return logWriter{t} }

func TestStartStopCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: use -tags integration without -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cli, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer cli.Close()

	root := t.TempDir()
	name := ContainerName(root)
	t.Logf("container %s", name)

	if err := EnsureImage(ctx, cli, "neo4j:5.26", "missing", testWriter(t)); err != nil {
		t.Fatalf("EnsureImage: %v", err)
	}
	if _, err := Start(ctx, cli, StartParams{
		RootAbs: root, Name: name, Image: "neo4j:5.26",
		User: "neo4j", Password: "integration-test-password-123",
		BoltPort: 17687, HTTPPort: 17474,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = Stop(context.Background(), cli, name, 10, true)
	})

	st, err := Inspect(ctx, cli, name)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running, got %+v", st)
	}

	if err := Stop(ctx, cli, name, 10, false); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, err = Inspect(ctx, cli, name)
	if err != nil {
		t.Fatalf("Inspect after stop: %v", err)
	}
	if st.Running {
		t.Fatalf("expected stopped, got %+v", st)
	}
}
