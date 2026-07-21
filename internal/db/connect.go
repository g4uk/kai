package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	MaxRetryBudget   = 30 * time.Second
	RetryCapInterval = 5 * time.Second
)

func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db connect: %w", err)
	}

	delay := 100 * time.Millisecond
	for {
		if ctx.Err() != nil {
			db.Close()
			return nil, fmt.Errorf("db connect: %w", ctx.Err())
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, RetryCapInterval)
		pingErr := db.PingContext(pingCtx)
		pingCancel()

		if pingErr == nil {
			return db, nil
		}

		if ctx.Err() != nil {
			db.Close()
			return nil, fmt.Errorf("db connect: %w", ctx.Err())
		}

		// Stop early if the remaining budget is less than the next delay.
		if deadline, ok := ctx.Deadline(); ok {
			if time.Until(deadline) < delay {
				db.Close()
				return nil, fmt.Errorf("db connect: %w", context.DeadlineExceeded)
			}
		}

		select {
		case <-ctx.Done():
			db.Close()
			return nil, fmt.Errorf("db connect: %w", ctx.Err())
		case <-time.After(delay):
			delay *= 2
			if delay > RetryCapInterval {
				delay = RetryCapInterval
			}
		}
	}
}
