package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/migrate"
	"github.com/mamersfo/neo/internal/neo"
)

func newDbCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "db",
		Short: "Database operations (reset, …)",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(newDbResetCmd())
	c.AddCommand(newDbDiffCmd())
	return c
}

func newDbResetCmd() *cobra.Command {
	var runSeed bool
	var timeoutSec int

	c := &cobra.Command{
		Use:   "reset",
		Short: "Wipe the database and replay migrations + seed",
		Long: `DETACH DELETE all data, drop constraints/indexes, replay
neo/migrations/*.cypher in order (recording _neo_migrations history),
then run neo/seed.cypher unless --seed=false or the file is missing.`,
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
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(timeoutSec)*time.Second)
			defer cancel()
			uri := neo.BoltURI("localhost", cfg.Neo.BoltPort)
			db, err := neo.NewClient(uri, cfg.Neo.User, cfg.Neo.Password)
			if err != nil {
				return NeoError(err)
			}
			defer db.Close(ctx)

			out := cmd.OutOrStdout()
			seedPath := filepath.Join(root, "neo", "seed.cypher")
			if err := migrate.Reset(ctx, db, cfg.MigrationsPath(root), seedPath, runSeed, out); err != nil {
				return NeoError(fmt.Errorf("bolt %s (is it running? neo start): %w", uri, err))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&runSeed, "seed", true, "run neo/seed.cypher after replay (use --seed=false to skip)")
	c.Flags().IntVar(&timeoutSec, "timeout", 120, "seconds for the whole reset operation")
	return c
}
