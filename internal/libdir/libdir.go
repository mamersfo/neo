// Package libdir resolves the neo/lib asset directory per ADR-002.
//
// Precedence: $NEO_LIB_DIR (dev/operator override) → <exe-dir>/lib
// (co-located with the installed binary) → "" (caller falls back to
// embedded templates). It never errors; missing means "use embedded".
package libdir

import (
	"os"
	"path/filepath"
)

// Dir returns the asset directory or "" when none is available.
func Dir() string {
	if v := os.Getenv("NEO_LIB_DIR"); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), "lib"); isDir(cand) {
			return cand
		}
	}
	return ""
}

// ReadFile reads name from the asset directory. ok=false when unavailable.
func ReadFile(name string) (data []byte, ok bool) {
	dir := Dir()
	if dir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, false
	}
	return b, true
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
