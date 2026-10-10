package migrate

import (
	"strings"
	"testing"
)

func TestConstraintStatement(t *testing.T) {
	cases := []struct {
		name string
		in   ConstraintInfo
		want string // substring; "" + ok=false means TODO expected
		ok   bool
	}{
		{"node uniqueness",
			ConstraintInfo{"c1", "UNIQUENESS", "NODE", []string{"Person"}, []string{"name"}},
			"CREATE CONSTRAINT `c1` IF NOT EXISTS FOR (x:`Person`) REQUIRE x.`name` IS UNIQUE;", true},
		{"rel uniqueness",
			ConstraintInfo{"c2", "UNIQUENESS", "RELATIONSHIP", []string{"KNOWS"}, []string{"since"}},
			"FOR ()-[x:`KNOWS`]-() REQUIRE x.`since` IS UNIQUE;", true},
		{"composite node key",
			ConstraintInfo{"c3", "NODE_KEY", "NODE", []string{"Person"}, []string{"a", "b"}},
			"REQUIRE (x.`a`, x.`b`) IS NODE KEY;", true},
		{"existence",
			ConstraintInfo{"c4", "NODE_PROPERTY_EXISTENCE", "NODE", []string{"Person"}, []string{"name"}},
			"IS NOT NULL;", true},
		{"unknown type",
			ConstraintInfo{"c5", "FUTURE_TYPE", "NODE", []string{"Person"}, []string{"name"}},
			"TODO(diff)", false},
		{"multi label ambiguous",
			ConstraintInfo{"c6", "UNIQUENESS", "NODE", []string{"A", "B"}, []string{"x"}},
			"TODO(diff)", false},
		{"no props",
			ConstraintInfo{"c7", "UNIQUENESS", "NODE", []string{"A"}, nil},
			"TODO(diff)", false},
	}
	for _, tc := range cases {
		stmt, ok := ConstraintStatement(tc.in)
		if ok != tc.ok {
			t.Errorf("%s: ok=%v want %v (%q)", tc.name, ok, tc.ok, stmt)
		}
		if !strings.Contains(stmt, tc.want) {
			t.Errorf("%s: stmt %q missing %q", tc.name, stmt, tc.want)
		}
	}
}

func TestIndexStatement(t *testing.T) {
	cases := []struct {
		name string
		in   IndexInfo
		want string
		ok   bool
	}{
		{"range",
			IndexInfo{Name: "i1", Type: "RANGE", Uniqueness: "NONUNIQUE", EntityType: "NODE", LabelsOrTypes: []string{"Person"}, Properties: []string{"name"}},
			"CREATE INDEX `i1` IF NOT EXISTS FOR (x:`Person`) ON (x.`name`);", true},
		{"fulltext",
			IndexInfo{Name: "i2", Type: "FULLTEXT", Uniqueness: "NONUNIQUE", EntityType: "NODE", LabelsOrTypes: []string{"Post"}, Properties: []string{"title", "body"}},
			"CREATE FULLTEXT INDEX `i2` IF NOT EXISTS FOR (x:`Post`) ON EACH [x.`title`, x.`body`];", true},
		{"lookup skipped",
			IndexInfo{Name: "i3", Type: "LOOKUP", EntityType: "NODE"},
			"auto-managed", false},
		{"unique constraint-backed",
			IndexInfo{Name: "i4", Type: "RANGE", OwningConstraint: "c1", EntityType: "NODE", LabelsOrTypes: []string{"Person"}, Properties: []string{"name"}},
			"CREATE CONSTRAINT instead", false},
		{"vector todo",
			IndexInfo{Name: "i5", Type: "VECTOR", EntityType: "NODE", LabelsOrTypes: []string{"Doc"}, Properties: []string{"emb"}},
			"TODO(diff)", false},
	}
	for _, tc := range cases {
		stmt, ok := IndexStatement(tc.in)
		if ok != tc.ok {
			t.Errorf("%s: ok=%v want %v (%q)", tc.name, ok, tc.ok, stmt)
		}
		if !strings.Contains(stmt, tc.want) {
			t.Errorf("%s: stmt %q missing %q", tc.name, stmt, tc.want)
		}
	}
}

func TestRenderDiffEmpty(t *testing.T) {
	body := RenderDiff(LiveSchema{}, "neo4j://localhost:7687")
	if !strings.Contains(body, "empty diff") {
		t.Errorf("empty schema should note empty diff:\n%s", body)
	}
	if !strings.Contains(body, "DATA is") {
		t.Errorf("header must state data exclusion:\n%s", body)
	}
}

func TestRenderDiffMixed(t *testing.T) {
	s := LiveSchema{
		Constraints: []ConstraintInfo{
			{"c1", "UNIQUENESS", "NODE", []string{"Person"}, []string{"name"}},
			{"cX", "WEIRD", "NODE", []string{"Person"}, []string{"name"}},
		},
		Indexes: []IndexInfo{
			{Name: "i1", Type: "RANGE", EntityType: "NODE", LabelsOrTypes: []string{"Person"}, Properties: []string{"age"}},
		},
	}
	body := RenderDiff(s, "neo4j://localhost:7687")
	for _, want := range []string{"CREATE CONSTRAINT `c1`", "TODO(diff)", "CREATE INDEX `i1`", "-- down"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered diff missing %q:\n%s", want, body)
		}
	}
	// Rendered body must be appliable-shaped: every non-comment, non-empty
	// line ends with ';'.
	for _, line := range strings.Split(body, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "--") {
			continue
		}
		if !strings.HasSuffix(l, ";") {
			t.Errorf("non-comment line %q must end with ';'", l)
		}
	}
}
