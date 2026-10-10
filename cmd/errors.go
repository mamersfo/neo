package cmd

import "errors"

// Exit codes per ADR-003.
const (
	ExitOK     = 0
	ExitConfig = 1
	ExitDocker = 2
	ExitNeo4j  = 3
)

// ExitError carries a process exit code with an error.
// Docker failures → 2, Neo4j/Cypher failures → 3, everything else → 1.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// DockerError wraps err as an exit-2 failure.
func DockerError(err error) *ExitError { return &ExitError{Code: ExitDocker, Err: err} }

// NeoError wraps err as an exit-3 failure.
func NeoError(err error) *ExitError { return &ExitError{Code: ExitNeo4j, Err: err} }

func exitCode(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitConfig
}
