//go:build integration

// Diff round-trip: build schema live → dump → wipe → apply dump → same schema.
// Run with: go test -tags integration ./internal/migrate -run TestDiffRoundTrip -v
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

func liveTestDB(t *testing.T, ctx context.Context, bolt, http int) (neo.Client, string) {
	t.Helper()
	cli, err := docker.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer cli.Close()
	root := t.TempDir()
	name := docker.ContainerName(root)
	if err := docker.EnsureImage(ctx, cli, "neo4j:5.26", "missing", io.Discard); err != nil {
		t.Fatalf("EnsureImage: %v", err)
	}
	if _, err := docker.Start(ctx, cli, docker.StartParams{
		RootAbs: root, Name: name, Image: "neo4j:5.26",
		User: "neo4j", Password: "diff-integration-pw",
		BoltPort: bolt, HTTPPort: http,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = docker.Stop(context.Background(), cli, name, 10, true) })
	uri := neo.BoltURI("localhost", bolt)
	if err := neo.WaitReady(ctx, uri, "neo4j", "diff-integration-pw", 3*time.Minute); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	db, err := neo.NewClient(uri, "neo4j", "diff-integration-pw")
	if err != nil {
		t.Fatalf("neo NewClient: %v", err)
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	return db, uri
}

func TestDiffRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: use -tags integration without -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, uri := liveTestDB(t, ctx, 18687, 18474)

	// Build distinctive live schema (constraint + plain index + data).
	for _, s := range []string{
		"CREATE CONSTRAINT widget_sku IF NOT EXISTS FOR (w:Widget) REQUIRE w.sku IS UNIQUE;",
		"CREATE INDEX widget_price_idx IF NOT EXISTS FOR (w:Widget) ON (w.price);",
		"MERGE (:Widget {sku: \"a\", price: 1});",
	} {
		if err := db.Exec(ctx, s, nil); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	schema, err := FetchLiveSchema(ctx, db)
	if err != nil {
		t.Fatalf("FetchLiveSchema: %v", err)
	}
	if len(schema.Constraints) != 1 || len(schema.Indexes) < 1 {
		t.Fatalf("unexpected dump: %+v", schema)
	}

	mig := t.TempDir()
	diffPath, err := WriteDiffFile(mig, schema, uri)
	if err != nil {
		t.Fatalf("WriteDiffFile: %v", err)
	}
	if base := filepath.Base(diffPath); len(base) < len("20060102150405_diff.cypher") {
		t.Fatalf("bad diff filename %q", base)
	}

	// Wipe everything, then replay ONLY the diff file (no seed).
	if err := db.Exec(ctx, "MATCH (n) DETACH DELETE n", nil); err != nil {
		t.Fatal(err)
	}
	if err := dropAll(ctx, db, "CONSTRAINT", "SHOW CONSTRAINTS YIELD name RETURN name AS name"); err != nil {
		t.Fatal(err)
	}
	if err := dropAll(ctx, db, "INDEX", "SHOW INDEXES YIELD name RETURN name AS name"); err != nil {
		t.Fatal(err)
	}
	if err := Reset(ctx, db, mig, filepath.Join(t.TempDir(), "no-seed.cypher"), false, io.Discard); err != nil {
		raw, _ := os.ReadFile(diffPath)
		t.Fatalf("Reset(diff) failed; diff was:\n%s\nerr: %v", raw, err)
	}

	after, err := FetchLiveSchema(ctx, db)
	if err != nil {
		t.Fatalf("re-dump: %v", err)
	}
	if len(after.Constraints) != 1 || after.Constraints[0].Name != "widget_sku" {
		t.Errorf("constraints after round-trip: %+v", after.Constraints)
	}
	found := false
	for _, ix := range after.Indexes {
		if ix.Name == "widget_price_idx" {
			found = true
		}
	}
	if !found {
		t.Errorf("widget_price_idx missing after round-trip: %+v", after.Indexes)
	}
	// Data must NOT come back (schema-only scope).
	rows, err := db.Query(ctx, "MATCH (w:Widget) RETURN count(w) AS c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := rows[0]["c"].(int64); c != 0 {
		t.Errorf("Widget count = %d, want 0 (diff is schema-only)", c)
	}
}
