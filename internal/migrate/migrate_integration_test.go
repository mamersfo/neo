//go:build integration

// Live-DB reset cycle against a real Neo4j container. Run with:
//
//	go test -tags integration ./internal/migrate -run TestResetCycle -v
package migrate

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mamersfo/neo/internal/docker"
	"github.com/mamersfo/neo/internal/neo"
)

func TestResetCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: use -tags integration without -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cli, err := docker.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer cli.Close()

	root := t.TempDir()
	name := docker.ContainerName(root)
	const bolt, http = 17687, 17474
	if err := docker.EnsureImage(ctx, cli, "neo4j:5.26", "missing", io.Discard); err != nil {
		t.Fatalf("EnsureImage: %v", err)
	}
	if _, err := docker.Start(ctx, cli, docker.StartParams{
		RootAbs: root, Name: name, Image: "neo4j:5.26",
		User: "neo4j", Password: "migrate-integration-pw",
		BoltPort: bolt, HTTPPort: http,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = docker.Stop(context.Background(), cli, name, 10, true) })

	uri := neo.BoltURI("localhost", bolt)
	if err := neo.WaitReady(ctx, uri, "neo4j", "migrate-integration-pw", 3*time.Minute); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	db, err := neo.NewClient(uri, "neo4j", "migrate-integration-pw")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer db.Close(ctx)

	mig := filepath.Join(root, "migrations")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(mig, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("20260101000000_a_schema.cypher",
		"-- schema only (no data statements allowed in this file)\nCREATE CONSTRAINT person_name IF NOT EXISTS FOR (p:Person) REQUIRE p.name IS UNIQUE;\n")
	write("20260102000000_b_data.cypher",
		"-- multi-statement data file\nMERGE (:Person {name: \"Ada\"});\nMERGE (:Person {name: \"Grace\"});\n")
	seed := filepath.Join(root, "seed.cypher")
	if err := os.WriteFile(seed, []byte("MERGE (:Person {name: \"Seedling\"});\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Junk proves the wipe.
	if err := db.Exec(ctx, "MERGE (:Junk {x: 1});", nil); err != nil {
		t.Fatalf("pre-seed junk: %v", err)
	}

	if err := Reset(ctx, db, mig, seed, true, io.Discard); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	count := func(label string) int {
		t.Helper()
		rows, err := db.Query(ctx, "MATCH (n:"+label+") RETURN count(n) AS c", nil)
		if err != nil {
			t.Fatalf("count %s: %v", label, err)
		}
		if len(rows) != 1 {
			t.Fatalf("count %s: %d rows", label, len(rows))
		}
		n, ok := rows[0]["c"].(int64)
		if !ok {
			t.Fatalf("count %s: %T %v", label, rows[0]["c"], rows[0]["c"])
		}
		return int(n)
	}
	if got := count("Person"); got != 3 {
		t.Errorf("Person count = %d, want 3 (2 migrations + seed)", got)
	}
	if got := count("Junk"); got != 0 {
		t.Errorf("Junk count = %d, want 0 (wipe failed)", got)
	}
	if got := count("_neo_migrations"); got != 2 {
		t.Errorf("history count = %d, want 2", got)
	}
	cons, err := db.Query(ctx, "SHOW CONSTRAINTS YIELD name RETURN name AS name", nil)
	if err != nil {
		t.Fatalf("SHOW CONSTRAINTS: %v", err)
	}
	if len(cons) != 1 {
		t.Errorf("constraints = %v, want [person_name]", cons)
	}

	// Second reset: idempotent.
	if err := Reset(ctx, db, mig, seed, true, io.Discard); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	if got := count("Person"); got != 3 {
		t.Errorf("after re-reset Person count = %d, want 3", got)
	}
}
