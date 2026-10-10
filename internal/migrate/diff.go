package migrate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mamersfo/neo/internal/neo"
)

// Diff scope (ADR-006, v1 honest stub): schema objects only. Graph data has
// no canonical DDL text form, so `db diff` dumps live constraints + indexes
// as an appliable migration with TODO markers where reconstruction is
// ambiguous. Cross-state comparison (--from migration vs live) is v2.

// ConstraintInfo is one row of SHOW CONSTRAINTS.
type ConstraintInfo struct {
	Name          string
	Type          string // UNIQUENESS, NODE_KEY, NODE_PROPERTY_EXISTENCE, …
	EntityType    string // NODE | RELATIONSHIP
	LabelsOrTypes []string
	Properties    []string
}

// IndexInfo is one row of SHOW INDEXES.
type IndexInfo struct {
	Name             string
	Type             string // RANGE, FULLTEXT, TEXT, POINT, VECTOR, LOOKUP
	Uniqueness       string // UNIQUE | NONUNIQUE (when reported)
	OwningConstraint string // non-empty when constraint-backed
	EntityType       string // NODE | RELATIONSHIP
	LabelsOrTypes    []string
	Properties       []string
}

// LiveSchema is the dumpable schema surface of a live database.
type LiveSchema struct {
	Constraints []ConstraintInfo
	Indexes     []IndexInfo
}

// FetchLiveSchema dumps constraints + indexes from the live database.
// Bare SHOW (no YIELD allowlist) keeps this working across 5.x/2025.x
// column renames; unknown columns are ignored, missing ones read as "".
func FetchLiveSchema(ctx context.Context, db neo.Client) (LiveSchema, error) {
	var s LiveSchema
	cons, err := db.Query(ctx, "SHOW CONSTRAINTS", nil)
	if err != nil {
		return s, fmt.Errorf("migrate: show constraints: %w", err)
	}
	for _, r := range cons {
		s.Constraints = append(s.Constraints, ConstraintInfo{
			Name:          str(r["name"]),
			Type:          str(r["type"]),
			EntityType:    str(r["entityType"]),
			LabelsOrTypes: strSlice(r["labelsOrTypes"]),
			Properties:    strSlice(r["properties"]),
		})
	}
	idxs, err := db.Query(ctx, "SHOW INDEXES", nil)
	if err != nil {
		return s, fmt.Errorf("migrate: show indexes: %w", err)
	}
	for _, r := range idxs {
		s.Indexes = append(s.Indexes, IndexInfo{
			Name:             str(r["name"]),
			Type:             str(r["type"]),
			Uniqueness:       str(r["uniqueness"]),
			OwningConstraint: str(r["owningConstraint"]),
			EntityType:       str(r["entityType"]),
			LabelsOrTypes:    strSlice(r["labelsOrTypes"]),
			Properties:       strSlice(r["properties"]),
		})
	}
	return s, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func strSlice(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = t
	case string:
		if t != "" {
			out = []string{t}
		}
	}
	return out
}

// ident backtick-quotes an identifier, doubling embedded backticks.
func ident(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// propRefs renders x.a, x.b for variable x.
func propRefs(v string, props []string) string {
	refs := make([]string, 0, len(props))
	for _, p := range props {
		refs = append(refs, v+"."+ident(p))
	}
	return strings.Join(refs, ", ")
}

// forPattern renders the FOR pattern for one label/type, or "" if ambiguous.
func forPattern(entityType string, labelsOrTypes []string) (pattern, variable string, ok bool) {
	if len(labelsOrTypes) != 1 {
		return "", "", false
	}
	lt := ident(labelsOrTypes[0])
	switch strings.ToUpper(entityType) {
	case "NODE":
		return "(x:" + lt + ")", "x", true
	case "RELATIONSHIP":
		return "()-[x:" + lt + "]-()", "x", true
	}
	return "", "", false
}

// ConstraintStatement renders CREATE CONSTRAINT, or (comment, false) with a
// TODO when the shape is ambiguous.
func ConstraintStatement(c ConstraintInfo) (stmt string, ok bool) {
	pat, v, good := forPattern(c.EntityType, c.LabelsOrTypes)
	if !good || len(c.Properties) == 0 {
		return todoConstraint(c), false
	}
	nm := "CONSTRAINT " + ident(c.Name) + " IF NOT EXISTS"
	switch {
	case contains(c.Type, "NODE_KEY") || contains(c.Type, "RELATIONSHIP_KEY") || contains(c.Type, " KEY"):
		if len(c.Properties) == 1 {
			return fmt.Sprintf("CREATE %s FOR %s REQUIRE %s IS %s;", nm, pat, propRefs(v, c.Properties), keyKind(c.Type)), true
		}
		return fmt.Sprintf("CREATE %s FOR %s REQUIRE (%s) IS %s;", nm, pat, propRefs(v, c.Properties), keyKind(c.Type)), true
	case contains(c.Type, "UNIQUENESS"):
		return fmt.Sprintf("CREATE %s FOR %s REQUIRE %s IS UNIQUE;", nm, pat, propRefs(v, c.Properties)), true
	case contains(c.Type, "EXISTENCE") || contains(c.Type, "NOT NULL") || contains(c.Type, "NOT_NULL"):
		if len(c.Properties) != 1 {
			return todoConstraint(c), false
		}
		return fmt.Sprintf("CREATE %s FOR %s REQUIRE %s IS NOT NULL;", nm, pat, propRefs(v, c.Properties)), true
	default:
		return todoConstraint(c), false
	}
}

func keyKind(t string) string {
	if contains(t, "RELATIONSHIP") {
		return "RELATIONSHIP KEY"
	}
	return "NODE KEY"
}

func todoConstraint(c ConstraintInfo) string {
	return fmt.Sprintf("-- TODO(diff): unrecognized constraint (type=%s entity=%s labels=%v props=%v name=%s); write CREATE CONSTRAINT manually.",
		c.Type, c.EntityType, c.LabelsOrTypes, c.Properties, c.Name)
}

// IndexStatement renders CREATE INDEX, (comment with skip=true) for LOOKUP
// (auto-managed), or (TODO, false) when ambiguous.
func IndexStatement(ix IndexInfo) (stmt string, appliable bool) {
	if strings.EqualFold(ix.Type, "LOOKUP") {
		return fmt.Sprintf("-- NOTE(diff): skipping %s lookup index %s (auto-managed, not creatable).", ix.EntityType, ident(ix.Name)), false
	}
	if strings.EqualFold(ix.Uniqueness, "UNIQUE") || ix.OwningConstraint != "" {
		return fmt.Sprintf("-- TODO(diff): %s is constraint-backed (owning constraint %s); express via CREATE CONSTRAINT instead of CREATE INDEX.", ident(ix.Name), ident(ix.OwningConstraint)), false
	}
	pat, v, good := forPattern(ix.EntityType, ix.LabelsOrTypes)
	if !good || len(ix.Properties) == 0 {
		return fmt.Sprintf("-- TODO(diff): ambiguous index (type=%s entity=%s labels=%v props=%v name=%s); write CREATE INDEX manually.",
			ix.Type, ix.EntityType, ix.LabelsOrTypes, ix.Properties, ix.Name), false
	}
	nm := "INDEX " + ident(ix.Name) + " IF NOT EXISTS"
	switch {
	case strings.EqualFold(ix.Type, "FULLTEXT"):
		return fmt.Sprintf("CREATE FULLTEXT %s FOR %s ON EACH [%s];", nm, pat, propRefs(v, ix.Properties)), true
	case strings.EqualFold(ix.Type, "RANGE") || strings.EqualFold(ix.Type, "TEXT") || strings.EqualFold(ix.Type, "POINT"):
		kind := strings.ToUpper(ix.Type)
		if kind == "RANGE" {
			return fmt.Sprintf("CREATE %s FOR %s ON (%s);", nm, pat, propRefs(v, ix.Properties)), true
		}
		return fmt.Sprintf("CREATE %s %s FOR %s ON (%s);", kind, nm, pat, propRefs(v, ix.Properties)), true
	default:
		return fmt.Sprintf("-- TODO(diff): unsupported index type=%s (name=%s); check Neo4j docs (e.g. VECTOR needs dimensions) and write manually.",
			ix.Type, ix.Name), false
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(strings.ToUpper(haystack), strings.ToUpper(needle))
}

// RenderDiff builds the migration file body for a live schema dump.
func RenderDiff(schema LiveSchema, source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- neo db diff (v1)\n-- Generated (UTC): %s\n-- Source: live %s\n--\n", time.Now().UTC().Format(time.RFC3339), source)
	b.WriteString("-- Scope: schema objects ONLY (constraints + indexes). Graph DATA is\n-- not included — seed/demo data belongs in seed.cypher or migrations.\n-- Review every statement below; ambiguous objects are commented TODOs.\n\n-- up\n\n")
	if len(schema.Constraints) == 0 && len(schema.Indexes) == 0 {
		b.WriteString("-- No constraints or indexes on the live database — empty diff.\n")
		return b.String()
	}
	if len(schema.Constraints) > 0 {
		b.WriteString("-- Constraints\n")
		for _, c := range schema.Constraints {
			stmt, _ := ConstraintStatement(c)
			b.WriteString(stmt + "\n")
		}
		b.WriteString("\n")
	}
	if len(schema.Indexes) > 0 {
		b.WriteString("-- Indexes\n")
		for _, ix := range schema.Indexes {
			stmt, _ := IndexStatement(ix)
			b.WriteString(stmt + "\n")
		}
	}
	b.WriteString("\n-- down (optional, unused in v1)\n")
	return b.String()
}

// WriteDiffFile renders the diff and writes neo/migrations/<ts>_diff.cypher.
func WriteDiffFile(migrationsPath string, schema LiveSchema, source string) (string, error) {
	if err := os.MkdirAll(migrationsPath, 0o755); err != nil {
		return "", fmt.Errorf("migrate: create dir: %w", err)
	}
	_, path, err := nextFilename(migrationsPath, "diff")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(RenderDiff(schema, source)), 0o644); err != nil {
		return "", fmt.Errorf("migrate: write %s: %w", path, err)
	}
	_ = os.Remove(migrationsPath + "/.gitkeep")
	return path, nil
}
