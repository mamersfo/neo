package docker

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
)

// Labels identifying containers managed by neo.
const (
	ManagedLabel = "managed-by"
	ManagedValue = "neo"
)

// Container ports Neo4j listens on inside the container.
const (
	BoltPort = "7687/tcp"
	HTTPPort = "7474/tcp"
)

// ErrNoContainer is returned when no neo container exists for the project.
var ErrNoContainer = errors.New("no neo container")

// Status is the observed state of a neo container (JSON tags for --output json).
type Status struct {
	Name         string `json:"name"`
	ID           string `json:"id"`
	Image        string `json:"image"`
	State        string `json:"state"`
	Running      bool   `json:"running"`
	BoltHostPort string `json:"bolt_host_port,omitempty"`
	HTTPHostPort string `json:"http_host_port,omitempty"`
}

// ContainerName derives a stable container name from the project root:
// "neo-" + first 12 hex chars of FNV-1a64(abs root).
func ContainerName(rootAbs string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(rootAbs))
	return fmt.Sprintf("neo-%.12x", h.Sum64())
}

// NewClient builds a Docker client from the environment (DOCKER_HOST,
// ~/.docker/*) with API version negotiation against the daemon.
func NewClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker: create client (is Docker running? DOCKER_HOST=%q): %w", os.Getenv("DOCKER_HOST"), err)
	}
	return cli, nil
}

// EnsureImage pulls ref if missing (policy "missing") or unconditionally
// (policy "always"). Policy "never" skips. Progress bodies are drained;
// human progress lines go to out.
func EnsureImage(ctx context.Context, cli *client.Client, ref, policy string, out io.Writer) error {
	switch policy {
	case "always", "missing", "never":
	default:
		return fmt.Errorf("docker: invalid --pull policy %q (missing|always|never)", policy)
	}
	if policy == "never" {
		return nil
	}
	if policy == "missing" {
		imgs, err := cli.ImageList(ctx, image.ListOptions{})
		if err != nil {
			return fmt.Errorf("docker: list images: %w", err)
		}
		for _, im := range imgs {
			for _, tag := range im.RepoTags {
				if tag == ref || tag == ref+":latest" {
					return nil
				}
			}
		}
	}
	fmt.Fprintf(out, "Pulling %s …\n", ref)
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker: pull %s: %w", ref, wrapDockerErr(err))
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("docker: pull %s: %w", ref, wrapDockerErr(err))
	}
	fmt.Fprintf(out, "Pulled %s\n", ref)
	return nil
}

// StartParams are the inputs to Start (from neo.toml + project root).
type StartParams struct {
	RootAbs  string
	Name     string
	Image    string
	User     string
	Password string
	BoltPort int
	HTTPPort int
}

// Start creates (or starts) the neo container. Idempotent: an already-running
// container is returned as-is.
func Start(ctx context.Context, cli *client.Client, p StartParams) (string, error) {
	st, err := Inspect(ctx, cli, p.Name)
	if err == nil {
		if st.Running {
			return p.Name, nil
		}
		if err := cli.ContainerStart(ctx, p.Name, container.StartOptions{}); err != nil {
			return "", fmt.Errorf("docker: start %s: %w", p.Name, wrapDockerErr(err))
		}
		return p.Name, nil
	}
	if !errors.Is(err, ErrNoContainer) {
		return "", err
	}

	cfg := &container.Config{
		Image: p.Image,
		Env: []string{
			"NEO4J_AUTH=" + p.User + "/" + p.Password,
			`NEO4J_PLUGINS=["apoc"]`,
		},
		ExposedPorts: nat.PortSet{BoltPort: {}, HTTPPort: {}},
		Labels: map[string]string{
			ManagedLabel: ManagedValue,
			"neo.root":   p.RootAbs,
		},
	}
	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			BoltPort: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: portNum(p.BoltPort)}},
			HTTPPort: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: portNum(p.HTTPPort)}},
		},
	}
	resp, err := cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, p.Name)
	if err != nil {
		return "", fmt.Errorf("docker: create %s: %w", p.Name, wrapDockerErr(err))
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("docker: start %s: %w", p.Name, wrapDockerErr(err))
	}
	return p.Name, nil
}

// Stop stops the container (idempotent). If remove is true it also removes it.
func Stop(ctx context.Context, cli *client.Client, name string, timeoutSec int, remove bool) error {
	st, err := Inspect(ctx, cli, name)
	if err != nil {
		if errors.Is(err, ErrNoContainer) {
			return ErrNoContainer
		}
		return err
	}
	if st.Running {
		if err := cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeoutSec}); err != nil {
			return fmt.Errorf("docker: stop %s: %w", name, wrapDockerErr(err))
		}
	}
	if remove {
		if err := cli.ContainerRemove(ctx, name, container.RemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("docker: remove %s: %w", name, wrapDockerErr(err))
		}
	}
	return nil
}

// Inspect returns the observed status, or ErrNoContainer if absent.
func Inspect(ctx context.Context, cli *client.Client, name string) (Status, error) {
	j, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return Status{}, fmt.Errorf("%w: %s", ErrNoContainer, name)
		}
		return Status{}, fmt.Errorf("docker: inspect %s: %w", name, wrapDockerErr(err))
	}
	st := Status{
		Name:  strings.TrimPrefix(j.Name, "/"),
		Image: imageOf(j.Config),
	}
	if j.State != nil {
		st.State = j.State.Status
		st.Running = j.State.Running
	}
	if j.NetworkSettings != nil {
		st.BoltHostPort = firstHostPort(j.NetworkSettings.Ports, BoltPort)
		st.HTTPHostPort = firstHostPort(j.NetworkSettings.Ports, HTTPPort)
	}
	if j.ID != "" {
		st.ID = shortID(j.ID)
	}
	return st, nil
}

func portNum(p int) string { return fmt.Sprintf("%d", p) }

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func firstHostPort(ports nat.PortMap, p nat.Port) string {
	for _, b := range ports[p] {
		if b.HostPort != "" {
			return b.HostPort
		}
	}
	return ""
}

func imageOf(cfg *container.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Image
}

// wrapDockerErr adds the "is Docker running?" hint to connection-level errors.
func wrapDockerErr(err error) error {
	if client.IsErrConnectionFailed(err) {
		return fmt.Errorf("%w (is Docker running? check DOCKER_HOST)", err)
	}
	return err
}
