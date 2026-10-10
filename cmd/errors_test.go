package cmd

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	if got := exitCode(fmt.Errorf("plain")); got != ExitConfig {
		t.Errorf("plain err = %d, want %d", got, ExitConfig)
	}
	if got := exitCode(DockerError(fmt.Errorf("x"))); got != ExitDocker {
		t.Errorf("docker err = %d, want %d", got, ExitDocker)
	}
	if got := exitCode(NeoError(fmt.Errorf("x"))); got != ExitNeo4j {
		t.Errorf("neo err = %d, want %d", got, ExitNeo4j)
	}
	// Wrapped ExitError must still map (errors.As through %w).
	wrapped := fmt.Errorf("start: %w", DockerError(errors.New("boom")))
	if got := exitCode(wrapped); got != ExitDocker {
		t.Errorf("wrapped docker err = %d, want %d", got, ExitDocker)
	}
}
