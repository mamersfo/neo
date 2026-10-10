package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
)

func newInitCmd() *cobra.Command {
	var dir string
	var force bool

	c := &cobra.Command{
		Use:   "init",
		Short: "Initialize a neo project (create neo/neo.toml, migrations, seed)",
		Long: `Create a neo/ directory in --dir (default .), safe to commit.
Generates a random Neo4j password stored in neo/neo.toml.
Idempotent: refuses to overwrite without --force.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := config.Init(dir, force)
			if err != nil {
				if errors.Is(err, config.ErrAlreadyInitialized) {
					return fmt.Errorf("%v\nHint: re-run with --force to overwrite", err)
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized neo project at %s\n", root)
			fmt.Fprintln(cmd.OutOrStdout(), "Next: neo start")
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", ".", "project directory to initialize")
	c.Flags().BoolVar(&force, "force", false, "overwrite existing neo/neo.toml")
	return c
}
