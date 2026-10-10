package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the `neo` root command. version is injected via ldflags
// at release time; defaults to dev in local builds.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "neo",
		Short: "Supabase-CLI-like tool for Neo4j (local Docker + Cypher migrations)",
		Long: `neo provisions a local Neo4j Community container and manages
Cypher migrations — init / start / status / stop / migration new / db reset / db diff.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.Flags().BoolP("help", "h", false, "help for neo")
	root.Version = version
	root.SetVersionTemplate("neo {{.Version}}\n")

	// Per ADR-003: exit codes 0 ok, 1 usage/config, 2 docker, 3 neo4j/cypher.
	// Cobra maps here; subcommands return coded errors (Phase 2+).
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return err
	})

	root.AddCommand(newInitCmd())
	root.AddCommand(newStartCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newMigrationCmd())
	root.AddCommand(newDbCmd())

	return root
}

// Execute runs the root command and maps errors to process exit codes.
func Execute(version string) int {
	root := NewRootCmd(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return exitCode(err)
	}
	return 0
}
