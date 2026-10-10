package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/docker"
	"github.com/mamersfo/neo/internal/neo"
)

func newStartCmd() *cobra.Command {
	var timeoutSec int
	var pull string

	c := &cobra.Command{
		Use:   "start",
		Short: "Start the local Neo4j container (pull image on first run)",
		Long: `Pull the pinned Neo4j image if missing, create (or start) the
project container, wait until Bolt is ready, then print local credentials.
The password is printed ONLY here — 'neo status' hides it in table mode.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			root, err := config.FindRoot(".")
			if err != nil {
				return err
			}
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			cli, err := docker.NewClient()
			if err != nil {
				return DockerError(err)
			}
			defer cli.Close()

			name := docker.ContainerName(root)
			out := cmd.OutOrStdout()
			if err := docker.EnsureImage(ctx, cli, cfg.Neo.Image, pull, out); err != nil {
				return DockerError(err)
			}
			if _, err := docker.Start(ctx, cli, docker.StartParams{
				RootAbs:  root,
				Name:     name,
				Image:    cfg.Neo.Image,
				User:     cfg.Neo.User,
				Password: cfg.Neo.Password,
				BoltPort: cfg.Neo.BoltPort,
				HTTPPort: cfg.Neo.HTTPPort,
			}); err != nil {
				return DockerError(err)
			}

			uri := neo.BoltURI("localhost", cfg.Neo.BoltPort)
			fmt.Fprintln(out, "Waiting for Bolt …")
			wt, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
			defer cancel()
			if err := neo.WaitReady(wt, uri, cfg.Neo.User, cfg.Neo.Password, time.Duration(timeoutSec)*time.Second); err != nil {
				return NeoError(fmt.Errorf("bolt %s: %w", uri, err))
			}

			fmt.Fprintf(out, "Started %s (%s)\n", name, cfg.Neo.Image)
			fmt.Fprintf(out, "Bolt: %s\n", uri)
			fmt.Fprintf(out, "HTTP: http://localhost:%d\n", cfg.Neo.HTTPPort)
			fmt.Fprintf(out, "User: %s\n", cfg.Neo.User)
			fmt.Fprintf(out, "Password: %s\n", cfg.Neo.Password)
			return nil
		},
	}
	c.Flags().IntVar(&timeoutSec, "timeout", 60, "seconds to wait for Bolt readiness")
	c.Flags().StringVar(&pull, "pull", "missing", "image pull policy: missing|always|never")
	return c
}
