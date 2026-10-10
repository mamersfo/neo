package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mamersfo/neo/internal/config"
	"github.com/mamersfo/neo/internal/docker"
	"github.com/mamersfo/neo/internal/neo"
)

// statusJSON is the --output json document. Password comes from neo.toml
// (config), never from Docker — table mode omits it.
type statusJSON struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	State    string `json:"state,omitempty"`
	Image    string `json:"image,omitempty"`
	BoltURL  string `json:"bolt_url"`
	HTTPURL  string `json:"http_url"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func newStatusCmd() *cobra.Command {
	var output string

	c := &cobra.Command{
		Use:   "status",
		Short: "Show local Neo4j status and credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output != "table" && output != "json" {
				return fmt.Errorf("invalid --output %q (table|json)", output)
			}
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
			st, err := docker.Inspect(cmd.Context(), cli, name)
			if err != nil {
				// Not created / removed: still exit 0, report not-running.
				if errors.Is(err, docker.ErrNoContainer) {
					return printStatus(out, output, statusJSON{
						Name:     name,
						Running:  false,
						BoltURL:  neo.BoltURI("localhost", cfg.Neo.BoltPort),
						HTTPURL:  fmt.Sprintf("http://localhost:%d", cfg.Neo.HTTPPort),
						User:     cfg.Neo.User,
						Password: cfg.Neo.Password,
					})
				}
				return DockerError(err)
			}

			boltPort := orDefault(st.BoltHostPort, itoa(cfg.Neo.BoltPort))
			httpPort := orDefault(st.HTTPHostPort, itoa(cfg.Neo.HTTPPort))
			return printStatus(out, output, statusJSON{
				Name:     st.Name,
				Running:  st.Running,
				State:    st.State,
				Image:    st.Image,
				BoltURL:  neo.BoltURI("localhost", atoi(boltPort, cfg.Neo.BoltPort)),
				HTTPURL:  "http://localhost:" + httpPort,
				User:     cfg.Neo.User,
				Password: cfg.Neo.Password,
			})
		},
	}
	c.Flags().StringVar(&output, "output", "table", "output format: table|json")
	return c
}

func printStatus(out interface{ Write([]byte) (int, error) }, format string, s statusJSON) error {
	if format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	fmt.Fprintf(out, "Name:  %s\n", s.Name)
	if s.State != "" {
		fmt.Fprintf(out, "State: %s\n", s.State)
	} else {
		fmt.Fprintf(out, "State: not running\n")
	}
	if s.Image != "" {
		fmt.Fprintf(out, "Image: %s\n", s.Image)
	}
	fmt.Fprintf(out, "Bolt:  %s\n", s.BoltURL)
	fmt.Fprintf(out, "HTTP:  %s\n", s.HTTPURL)
	fmt.Fprintf(out, "User:  %s\n", s.User)
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

func atoi(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}
