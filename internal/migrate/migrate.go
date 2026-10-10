package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/mamersfo/neo/internal/libdir"
	"github.com/mamersfo/neo/internal/neo"
)

// HistoryLabel is the node label recording applied migrations.
const HistoryLabel = "_neo_migrations"

// migrationTemplate is the compiled-in copy of neo/lib/migration.tmpl.cypher
// (drift test pins them equal); a same-named lib/ file overrides at runtime.
// NOTE: keep schema statements (CREATE CONSTRAINT/INDEX) in data-free files:
// one statement runs per transaction and mixing is fragile.
const migrationTemplate = `-- neo migration: {{.Filename}}
-- Created (UTC): {{.Timestamp}}
--
-- Write Cypher below. Multiple data statements allowed (separated by ;).
-- Schema statements (CONSTRAINT/INDEX) must live in data-free files.

-- up

-- e.g.:
-- MERGE (p:Person {name: "Ada"});

-- down (optional, unused in v1)
`

func loadMigrationTemplate() (*template.Template, error) {
	if b, ok := libdir.ReadFile("migration.tmpl.cypher"); ok {
		return template.New("migration").Parse(string(b))
	}
	return template.New("migration").Parse(migrationTemplate)
}

// Entry is one migration file on disk.
type Entry struct {
	Filename string
	Path     string
	Checksum string // sha256 hex of raw file bytes
}

// Slugify maps a human name to [a-z0-9_]: "Create Person!" → "create_person".
func Slugify(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	prevUnderscore := true // trim leading separators
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// Timestamp returns UTC %Y%m%d%H%M%S for filename prefixes.
func Timestamp() string { return time.Now().UTC().Format("20060102150405") }

// nextFilename returns a free <ts>_<slug>.cypher path, bumping the timestamp
// on same-second collisions so creation order == lexicographic order.
func nextFilename(migrationsPath, slug string) (filename, path string, err error) {
	base := time.Now().UTC()
	for i := 0; i < 3600; i++ {
		filename = base.Add(time.Duration(i)*time.Second).Format("20060102150405") + "_" + slug + ".cypher"
		path = filepath.Join(migrationsPath, filename)
		if _, serr := os.Stat(path); os.IsNotExist(serr) {
			return filename, path, nil
		}
	}
	return "", "", fmt.Errorf("migrate: cannot find free timestamp for %q", slug)
}

// New creates neo/migrations/<ts>_<slug>.cypher and returns its path.
func New(migrationsPath, name string) (string, error) {
	slug := Slugify(name)
	if slug == "" {
		return "", fmt.Errorf("migrate: name %q yields empty filename (use letters/digits)", name)
	}
	if err := os.MkdirAll(migrationsPath, 0o755); err != nil {
		return "", fmt.Errorf("migrate: create dir: %w", err)
	}
	// Same-second creation must still order deterministically: bump the
	// timestamp until the filename is free (supabase-style %Y%m%d%H%M%S).
	filename, path, err := nextFilename(migrationsPath, slug)
	if err != nil {
		return "", err
	}
	t, err := loadMigrationTemplate()
	if err != nil {
		return "", fmt.Errorf("migrate: parse template: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("migrate: write %s: %w", path, err)
	}
	defer f.Close()
	data := map[string]string{
		"Filename":  filename,
		"Timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if err := t.Execute(f, data); err != nil {
		return "", fmt.Errorf("migrate: render %s: %w", path, err)
	}
	// Remove a stale .gitkeep now that a real migration exists.
	_ = os.Remove(filepath.Join(migrationsPath, ".gitkeep"))
	return path, nil
}

// List returns *.cypher entries sorted lexicographically (== chronological).
func List(migrationsPath string) ([]Entry, error) {
	dir, err := os.ReadDir(migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", migrationsPath, err)
	}
	var out []Entry
	for _, d := range dir {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".cypher") {
			continue
		}
		path := filepath.Join(migrationsPath, d.Name())
		sum, err := ChecksumFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{Filename: d.Name(), Path: path, Checksum: sum})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Filename < out[j].Filename })
	return out, nil
}

// ChecksumFile returns sha256 hex of a file.
func ChecksumFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("migrate: read %s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// strippable reports whether a line is a comment or blank.
func isCommentOrBlank(line string) bool {
	t := strings.TrimSpace(line)
	return t == "" || strings.HasPrefix(t, "--") || strings.HasPrefix(t, "//")
}

// SplitStatements splits Cypher source on top-level semicolons, ignoring
// semicolons inside '...', "...", `...`, line comments (--, //) and
// /* block comments */. Comment text is dropped. The Bolt driver accepts
// exactly one statement per query, so multi-statement files need this.
func SplitStatements(body string) []string {
	var stmts []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			stmts = append(stmts, s)
		}
		cur.Reset()
	}
	const (
		normal = iota
		inSingle
		inDouble
		inBacktick
		lineComment
		blockComment
	)
	state := normal
	runes := []rune(body)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}
		switch state {
		case normal:
			switch {
			case r == '\'':
				state = inSingle
				cur.WriteRune(r)
			case r == '"':
				state = inDouble
				cur.WriteRune(r)
			case r == '`':
				state = inBacktick
				cur.WriteRune(r)
			case r == '-' && next == '-':
				state = lineComment
				i++ // consume second -
			case r == '/' && next == '/':
				state = lineComment
				i++
			case r == '/' && next == '*':
				state = blockComment
				i++
			case r == ';':
				flush()
			default:
				cur.WriteRune(r)
			}
		case inSingle, inDouble, inBacktick:
			quote := []rune{'\'', '"', '`'}[state-1]
			cur.WriteRune(r)
			if r == '\\' && i+1 < len(runes) {
				i++
				cur.WriteRune(runes[i])
			} else if r == quote {
				// '' is an escaped quote inside a string, not the end.
				if next == quote {
					i++
					cur.WriteRune(next)
				} else {
					state = normal
				}
			}
		case lineComment:
			if r == '\n' {
				state = normal
				cur.WriteRune(r)
			}
		case blockComment:
			if r == '*' && next == '/' {
				state = normal
				i++
			}
		}
	}
	flush()
	return stmts
}

// StrippedBody returns the file minus comment/blank lines. Empty body means
// "nothing to execute" (e.g. a fresh template) — callers skip, not fail.
func StrippedBody(content string) string {
	var keep []string
	for _, line := range strings.Split(content, "\n") {
		if !isCommentOrBlank(line) {
			keep = append(keep, line)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// Reset wipes the database (data, constraints, indexes, history), replays
// migrations in order, records history, then runs seed unless skipped.
// Seed runs only if seedPath exists with a non-empty body.
func Reset(ctx context.Context, db neo.Client, migrationsPath, seedPath string, runSeed bool, out io.Writer) error {
	entries, err := List(migrationsPath)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Wiping database …")
	if err := db.Exec(ctx, "MATCH (n) DETACH DELETE n", nil); err != nil {
		return fmt.Errorf("migrate: wipe data: %w", err)
	}
	if err := dropAll(ctx, db, "CONSTRAINT", "SHOW CONSTRAINTS YIELD name RETURN name AS name"); err != nil {
		return err
	}
	if err := dropAll(ctx, db, "INDEX", "SHOW INDEXES YIELD name RETURN name AS name"); err != nil {
		return err
	}

	for _, e := range entries {
		raw, err := os.ReadFile(e.Path)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", e.Filename, err)
		}
		body := StrippedBody(string(raw))
		stmts := SplitStatements(body)
		if len(stmts) == 0 {
			fmt.Fprintf(out, "Skipping empty %s (template — fill it in)\n", e.Filename)
			continue
		}
		fmt.Fprintf(out, "Applying %s (%d statement(s)) …\n", e.Filename, len(stmts))
		for _, s := range stmts {
			if err := db.Exec(ctx, s, nil); err != nil {
				return fmt.Errorf("migrate: apply %s: %w", e.Filename, err)
			}
		}
		if err := db.Exec(ctx,
			"MERGE (m:"+HistoryLabel+" {filename: $f}) SET m.checksum = $c, m.applied_at = datetime()",
			map[string]any{"f": e.Filename, "c": e.Checksum}); err != nil {
			return fmt.Errorf("migrate: record %s: %w", e.Filename, err)
		}
	}

	if runSeed {
		raw, err := os.ReadFile(seedPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintln(out, "No seed.cypher — skipping seed.")
				return nil
			}
			return fmt.Errorf("migrate: read seed: %w", err)
		}
		if body := StrippedBody(string(raw)); body == "" {
			fmt.Fprintln(out, "Seed empty — skipping.")
		} else {
			fmt.Fprintln(out, "Seeding …")
			if err := db.Exec(ctx, body, nil); err != nil {
				return fmt.Errorf("migrate: seed: %w", err)
			}
		}
	}
	fmt.Fprintln(out, "Reset complete.")
	return nil
}

func dropAll(ctx context.Context, db neo.Client, kind, showCypher string) error {
	rows, err := db.Query(ctx, showCypher, nil)
	if err != nil {
		return fmt.Errorf("migrate: list %ss: %w", kind, err)
	}
	for _, r := range rows {
		name, _ := r["name"].(string)
		if name == "" {
			continue
		}
		if err := db.Exec(ctx, "DROP "+kind+" `"+name+"`", nil); err != nil {
			return fmt.Errorf("migrate: drop %s %s: %w", kind, name, err)
		}
	}
	return nil
}
