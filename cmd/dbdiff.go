package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/migrate"
	"github.com/mamersfo/neo/internal/neo"
)

func newDbDiffCmd() *cobra.Command {
	var output string
	var timeoutSec int

	c := &cobra.Command{
		Use:   "diff",
		Short: "Dump live schema (constraints + indexes) as a new migration",
		Long: `v1 honest scope: dumps the LIVE database schema (SHOW CONSTRAINTS /
SHOW INDEXES) into a new neo/migrations/<ts>_diff.cypher file.

Limitations (by design, see design.md ADR-006):
- Graph DATA is not included — only schema objects.
- Ambiguous objects become commented TODOs for manual review.
- No --from/--to state comparison yet (v2); this is the --from empty
  --to live dump. The file is appliable via 'neo db reset'.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := config.FindRoot(".")
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			uri := neo.BoltURI("localhost", cfg.Neo.BoltPort)
			db, err := neo.NewClient(uri, cfg.Neo.User, cfg.Neo.Password)
			if err != nil {
				return NeoError(err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(timeoutSec)*time.Second)
			defer cancel()
			defer db.Close(ctx)

			schema, err := migrate.FetchLiveSchema(ctx, db)
			if err != nil {
				return NeoError(fmt.Errorf("bolt %s (is it running? neo start): %w", uri, err))
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Live schema: %d constraint(s), %d index(es)\n", len(schema.Constraints), len(schema.Indexes))
			if output != "" {
				content := migrate.RenderDiff(schema, uri)
				if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", output, err)
				}
				fmt.Fprintf(out, "Wrote %s\n", output)
				return nil
			}
			path, err := migrate.WriteDiffFile(cfg.MigrationsPath(root), schema, uri)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Wrote %s\n", path)
			fmt.Fprintln(out, "Review TODOs inside, then 'neo db reset' to verify it applies cleanly.")
			return nil
		},
	}
	c.Flags().StringVar(&output, "output", "", "write to this path instead of a new migration file")
	c.Flags().IntVar(&timeoutSec, "timeout", 60, "seconds for the schema dump")
	return c
}
