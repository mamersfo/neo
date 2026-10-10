package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/docker"
)

func newStopCmd() *cobra.Command {
	var timeoutSec int
	var remove bool

	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Neo4j container",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := config.FindRoot(".")
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
			if err := docker.Stop(cmd.Context(), cli, name, timeoutSec, remove); err != nil {
				if errors.Is(err, docker.ErrNoContainer) {
					fmt.Fprintf(out, "%s: already stopped (no container)\n", name)
					return nil
				}
				return DockerError(err)
			}
			if remove {
				fmt.Fprintf(out, "Stopped and removed %s\n", name)
			} else {
				fmt.Fprintf(out, "Stopped %s\n", name)
			}
			return nil
		},
	}
	c.Flags().IntVar(&timeoutSec, "timeout", 10, "seconds to wait for graceful stop")
	c.Flags().BoolVar(&remove, "remove", false, "remove the container after stopping")
	return c
}
