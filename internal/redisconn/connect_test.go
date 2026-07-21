package redisconn

import (
	"context"
	"testing"
	"time"
)

func TestConnect_InvalidAddr(t *testing.T) {
	// Port 1 on loopback refuses connections; expect error within budget.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	start := time.Now()
	_, err := Connect(ctx, "127.0.0.1:1")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected non-nil error for unreachable Redis addr, got nil")
	}
	if elapsed >= 6*time.Second {
		t.Errorf("Connect did not respect context deadline: took %v", elapsed)
	}
}

func TestConnectRetryConfig(t *testing.T) {
	if MaxRetryBudget < 25*time.Second {
		t.Errorf("MaxRetryBudget = %v, want >= 25s", MaxRetryBudget)
	}
	if RetryCapInterval > 6*time.Second {
		t.Errorf("RetryCapInterval = %v, want <= 6s", RetryCapInterval)
	}
}
