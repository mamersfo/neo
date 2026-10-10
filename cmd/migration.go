package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/migrate"
)

func newMigrationCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "migration",
		Short: "Manage Cypher migrations",
		Args:  cobra.NoArgs,
	}
	c.AddCommand(newMigrationNewCmd())
	return c
}

func newMigrationNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new migration file",
		Long: `Create neo/migrations/<UTC-timestamp>_<slug>.cypher from the template.
Example: neo migration new "create person"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := config.FindRoot(".")
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			path, err := migrate.New(cfg.MigrationsPath(root), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
}
