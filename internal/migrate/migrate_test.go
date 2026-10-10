package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Create Person":      "create_person",
		"create-person!":     "create_person",
		"  Add  INDEX x  ":   "add_index_x",
		"ALLCAPS":            "allcaps",
		"a/b\\c:d":           "a_b_c_d",
		"ünïcode → ascii?":   "n_code_ascii",
		"already_snake_case": "already_snake_case",
		"123 numbers ok":     "123_numbers_ok",
		"":                   "",
		"---":                "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewCreatesFile(t *testing.T) {
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	// stale .gitkeep should be removed once a real migration exists
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mig, ".gitkeep"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	before := Timestamp()
	path, err := New(mig, "Create Person")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	after := Timestamp()

	base := filepath.Base(path)
	if !strings.HasSuffix(base, "_create_person.cypher") {
		t.Errorf("filename = %q, want suffix _create_person.cypher", base)
	}
	ts := strings.TrimSuffix(base, "_create_person.cypher")
	if len(ts) != 14 || ts < before || ts > after {
		t.Errorf("timestamp prefix = %q (range %q–%q)", ts, before, after)
	}
	content, _ := os.ReadFile(path)
	for _, want := range []string{"-- up", "-- down", base} {
		if !strings.Contains(string(content), want) {
			t.Errorf("template missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(mig, ".gitkeep")); !os.IsNotExist(err) {
		t.Error(".gitkeep should be removed after first real migration")
	}
}

func TestNewEmptyName(t *testing.T) {
	if _, err := New(t.TempDir(), "---"); err == nil {
		t.Error("expected error for empty slug")
	}
}

func TestNewSameSecondCollisionBumps(t *testing.T) {
	mig := t.TempDir()
	first, err := New(mig, "same name")
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	second, err := New(mig, "same name")
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if first == second {
		t.Errorf("collision: both %q", first)
	}
	if filepath.Base(second) <= filepath.Base(first) {
		t.Errorf("ordering: %q should sort after %q", filepath.Base(second), filepath.Base(first))
	}
}

func TestListOrderingAndChecksum(t *testing.T) {
	mig := t.TempDir()
	files := []string{"20260103000000_c.cypher", "20260101000000_a.cypher", "20260102000000_b.cypher", "notes.txt", ".gitkeep"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(mig, f), []byte("RETURN 1; // "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := List(mig)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3 (.cypher only)", len(entries))
	}
	want := []string{"20260101000000_a.cypher", "20260102000000_b.cypher", "20260103000000_c.cypher"}
	for i, w := range want {
		if entries[i].Filename != w {
			t.Errorf("entries[%d] = %q, want %q", i, entries[i].Filename, w)
		}
		if entries[i].Checksum == "" {
			t.Errorf("entries[%d] missing checksum", i)
		}
	}
	if entries[0].Checksum == entries[1].Checksum {
		t.Error("distinct files share a checksum")
	}
}

func TestChecksumChangesOnEdit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.cypher")
	os.WriteFile(p, []byte("RETURN 1;"), 0o644)
	a, _ := ChecksumFile(p)
	os.WriteFile(p, []byte("RETURN 2;"), 0o644)
	b, _ := ChecksumFile(p)
	if a == b {
		t.Error("checksum unchanged after edit")
	}
}

func TestStrippedBody(t *testing.T) {
	template := "-- neo migration: x\n-- up\n\n-- example\n// comment\n"
	if got := StrippedBody(template); got != "" {
		t.Errorf("template body = %q, want empty", got)
	}
	real := "-- comment\nMERGE (p:Person {name: \"Ada\"});\n\n// trailing\n"
	if got := StrippedBody(real); got != `MERGE (p:Person {name: "Ada"});` {
		t.Errorf("body = %q", got)
	}
}

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single", `MERGE (p:Person {name: "Ada"});`, []string{`MERGE (p:Person {name: "Ada"})`}},
		{"multi", "MERGE (:A);\nMERGE (:B);", []string{"MERGE (:A)", "MERGE (:B)"}},
		{"no trailing semi", "MERGE (:A)", []string{"MERGE (:A)"}},
		{"semi in single quotes", `MERGE (:A {note: 'a;b'});`, []string{`MERGE (:A {note: 'a;b'})`}},
		{"semi in double quotes", `MERGE (:A {note: "a;b"});`, []string{`MERGE (:A {note: "a;b"})`}},
		{"semi in backticks", "MATCH (`we;ird`) RETURN 1;", []string{"MATCH (`we;ird`) RETURN 1"}},
		{"escaped quote", `MERGE (:A {note: 'it''s; ok'});`, []string{`MERGE (:A {note: 'it''s; ok'})`}},
		{"line comment semi", "MERGE (:A); -- traili;ng\nMERGE (:B);", []string{"MERGE (:A)", "MERGE (:B)"}},
		{"block comment semi", "MERGE (:A) /* a;b */ RETURN 1;", []string{"MERGE (:A)  RETURN 1"}},
		{"empty", "-- only comments\n// nothing\n", nil},
	}
	for _, tc := range cases {
		got := SplitStatements(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s[%d]: got %q, want %q", tc.name, i, got[i], tc.want[i])
			}
		}
	}
}

func TestNewUsesLibDirTemplateOverride(t *testing.T) {
	lib := t.TempDir()
	if err := os.WriteFile(filepath.Join(lib, "migration.tmpl.cypher"),
		[]byte("-- lib-marker-789 {{.Filename}}\nMERGE (:M);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEO_LIB_DIR", lib)
	path, err := New(t.TempDir(), "lib override")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "lib-marker-789") {
		t.Errorf("expected lib override marker in %s:\n%s", path, raw)
	}
}

func TestMigrationTemplateMatchesLib(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "neo", "lib", "migration.tmpl.cypher"))
	if err != nil {
		t.Fatalf("read lib template: %v", err)
	}
	if string(raw) != migrationTemplate {
		t.Error("migrationTemplate const drifted from neo/lib/migration.tmpl.cypher — update both")
	}
}
