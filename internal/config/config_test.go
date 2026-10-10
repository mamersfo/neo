package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	root, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, p := range []string{
		filepath.Join(root, "neo", "neo.toml"),
		filepath.Join(root, "neo", "seed.cypher"),
		filepath.Join(root, "neo", "migrations", ".gitkeep"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Neo.Image != DefaultImage {
		t.Errorf("image = %q, want %q", cfg.Neo.Image, DefaultImage)
	}
	if len(cfg.Neo.Password) < 8 {
		t.Errorf("password too short: %q", cfg.Neo.Password)
	}
	if cfg.Neo.BoltPort != DefaultBoltPort || cfg.Neo.HTTPPort != DefaultHTTPPort {
		t.Errorf("ports = %d/%d, want %d/%d", cfg.Neo.BoltPort, cfg.Neo.HTTPPort, DefaultBoltPort, DefaultHTTPPort)
	}
}

func TestInitIdempotentWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir, false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(dir, false); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second Init err = %v, want ErrAlreadyInitialized", err)
	}
}

func TestInitForceOverwritesAndRotatesPassword(t *testing.T) {
	dir := t.TempDir()
	root, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	first, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := Init(dir, true); err != nil {
		t.Fatalf("Init --force: %v", err)
	}
	second, err := Load(root)
	if err != nil {
		t.Fatalf("Load after force: %v", err)
	}
	if first.Neo.Password == second.Neo.Password {
		t.Error("expected password rotation on --force, got identical passwords")
	}
}

func TestFindRoot(t *testing.T) {
	dir := t.TempDir()
	root, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if found != root {
		t.Errorf("FindRoot = %q, want %q", found, root)
	}
	if _, err := FindRoot(t.TempDir()); !errors.Is(err, ErrNotAProject) {
		t.Errorf("FindRoot outside project err = %v, want ErrNotAProject", err)
	}
}

func TestGeneratePasswordUnique(t *testing.T) {
	a, _ := GeneratePassword()
	b, _ := GeneratePassword()
	if a == b {
		t.Error("expected unique passwords")
	}
	if len(a) < 8 {
		t.Errorf("password too short: %q", a)
	}
}

func TestInitUsesLibDirTemplateOverride(t *testing.T) {
	lib := t.TempDir()
	// Custom operator template: must still parse with *Config data.
	tmpl := "# custom-marker-456\n[neo]\nimage = \"{{.Neo.Image}}\"\nbolt_port = {{.Neo.BoltPort}}\nhttp_port = {{.Neo.HTTPPort}}\nuser = \"{{.Neo.User}}\"\npassword = \"{{.Neo.Password}}\"\n[migrations]\ndir = \"{{.Migrations.Dir}}\"\n"
	if err := os.WriteFile(filepath.Join(lib, "neo.toml.tmpl"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEO_LIB_DIR", lib)
	root, err := Init(t.TempDir(), false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "neo", "neo.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "custom-marker-456") {
		t.Errorf("expected lib override marker in neo.toml:\n%s", raw)
	}
	if _, err := Load(root); err != nil {
		t.Errorf("Load(custom template): %v", err)
	}
}

func TestTomlTemplateMatchesLib(t *testing.T) {
	// go:embed cannot reach ../../, so the const duplicates the lib file.
	// This test fails the build on drift (single source of truth by test).
	raw, err := os.ReadFile(filepath.Join("..", "..", "neo", "lib", "neo.toml.tmpl"))
	if err != nil {
		t.Fatalf("read lib template: %v", err)
	}
	if string(raw) != tomlTemplate {
		t.Error("tomlTemplate const drifted from neo/lib/neo.toml.tmpl — update both")
	}
}
