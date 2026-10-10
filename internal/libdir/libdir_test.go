package libdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileEnvOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.tmpl"), []byte("marker-123"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEO_LIB_DIR", dir)
	if got := Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
	b, ok := ReadFile("x.tmpl")
	if !ok || string(b) != "marker-123" {
		t.Errorf("ReadFile = %q, %v", b, ok)
	}
	if _, ok := ReadFile("missing.tmpl"); ok {
		t.Error("missing file should report ok=false")
	}
}

func TestDirEmptyWithoutEnv(t *testing.T) {
	t.Setenv("NEO_LIB_DIR", "")
	// Either "" or a real co-located lib (release layout); both are valid,
	// but ReadFile of a bogus name must miss either way.
	if _, ok := ReadFile("definitely-not-here-*.tmpl"); ok {
		t.Error("bogus name should miss")
	}
}
