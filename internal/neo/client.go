package neo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ErrNotReady is returned by WaitReady when Bolt never becomes reachable.
var ErrNotReady = errors.New("neo4j not ready")

// Client is the ADR-006 seam: all Cypher goes through this interface so the
// driver can be swapped in one file. Backed by the official v5 driver.
type Client interface {
	// Exec runs cypher (one or more statements) in a write transaction.
	Exec(ctx context.Context, cypher string, params map[string]any) error
	// Query runs cypher in a read transaction, returning rows as maps.
	Query(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error)
	Close(ctx context.Context) error
}

type driverClient struct {
	driver neo4j.DriverWithContext
}

// DatabaseName is the target database (Community Edition has only "neo4j").
const DatabaseName = "neo4j"

// NewClient opens a Bolt driver. Connection is lazy — call Check/WaitReady.
func NewClient(uri, user, password string) (Client, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		return nil, fmt.Errorf("neo4j: create driver: %w", err)
	}
	return &driverClient{driver: driver}, nil
}

func (c *driverClient) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

func (c *driverClient) Exec(ctx context.Context, cypher string, params map[string]any) error {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: DatabaseName,
		AccessMode:   neo4j.AccessModeWrite,
	})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if _, err := res.Consume(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (c *driverClient) Query(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error) {
	sess := c.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: DatabaseName,
		AccessMode:   neo4j.AccessModeRead,
	})
	defer sess.Close(ctx)
	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		records, err := res.Collect(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			row := make(map[string]any, len(rec.Keys))
			for i, k := range rec.Keys {
				row[k] = rec.Values[i]
			}
			rows = append(rows, row)
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return out.([]map[string]any), nil
}

// BoltURI builds a neo4j:// URI for host:port.
func BoltURI(host string, port int) string {
	return fmt.Sprintf("neo4j://%s:%d", host, port)
}

// Check verifies Bolt connectivity once (valid credentials, reachable server).
func Check(ctx context.Context, uri, user, password string) error {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		return fmt.Errorf("neo4j: create driver: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j: verify connectivity: %w", err)
	}
	return nil
}

// WaitReady polls VerifyConnectivity every second until timeout.
// A cancelled ctx aborts early with ctx.Err().
func WaitReady(ctx context.Context, uri, user, password string, timeout time.Duration) error {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		return fmt.Errorf("neo4j: create driver: %w", err)
	}
	defer driver.Close(ctx)

	deadline := time.Now().Add(timeout)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		verr := driver.VerifyConnectivity(attempt)
		cancel()
		if verr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w after %s (last: %v)", ErrNotReady, timeout, verr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
