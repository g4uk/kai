package redisconn

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	MaxRetryBudget   = 30 * time.Second
	RetryCapInterval = 5 * time.Second
)

func Connect(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		MaxRetries:  -1, // disable internal retries; we handle back-off ourselves
		DialTimeout: 500 * time.Millisecond,
	})

	delay := 100 * time.Millisecond
	for {
		if ctx.Err() != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis connect: %w", ctx.Err())
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		pingErr := client.Ping(pingCtx).Err()
		pingCancel()

		if pingErr == nil {
			return client, nil
		}

		if ctx.Err() != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis connect: %w", ctx.Err())
		}

		// Stop early if remaining budget is less than the next delay.
		if deadline, ok := ctx.Deadline(); ok {
			if time.Until(deadline) < delay {
				_ = client.Close()
				return nil, fmt.Errorf("redis connect: %w", context.DeadlineExceeded)
			}
		}

		select {
		case <-ctx.Done():
			_ = client.Close()
			return nil, fmt.Errorf("redis connect: %w", ctx.Err())
		case <-time.After(delay):
			delay *= 2
			if delay > RetryCapInterval {
				delay = RetryCapInterval
			}
		}
	}
}
