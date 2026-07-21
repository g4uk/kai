package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// This test file drives internal/session/session.go, which does not exist yet
// (specs/user-auth/plan.md step 4). Expect a compile error until session.go
// defines:
//
//	type Store struct {
//	    Client *redis.Client
//	    TTL    time.Duration
//	}
//	func (s *Store) Create(ctx context.Context, userID uint64) (string, error)
//	func (s *Store) Validate(ctx context.Context, sessionID string) (uint64, error)
//	func (s *Store) Delete(ctx context.Context, sessionID string) error
//	var ErrNotFound error
//
// Redis key layout under test (per plan.md): session:{sessionID}.

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping integration test")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { client.Close() })

	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	return client
}

func newTestStore(client *redis.Client) *Store {
	return &Store{
		Client: client,
		TTL:    30 * 24 * time.Hour,
	}
}

func TestCreateValidate_RoundTrip(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	store := newTestStore(client)

	const userID uint64 = 42

	sessionID, err := store.Create(ctx, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sessionID == "" {
		t.Fatal("Create returned empty sessionID")
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), fmt.Sprintf("session:%s", sessionID)).Err() })

	got, err := store.Validate(ctx, sessionID)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got != userID {
		t.Errorf("Validate returned userID %d, want %d", got, userID)
	}
}

func TestValidate_UnknownID(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	store := newTestStore(client)

	if _, err := store.Validate(ctx, "no-such-session-id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Validate unknown ID: got %v, want ErrNotFound", err)
	}
}

func TestDeleteThenValidate(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	store := newTestStore(client)

	sessionID, err := store.Create(ctx, 7)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), fmt.Sprintf("session:%s", sessionID)).Err() })

	if err := store.Delete(ctx, sessionID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := store.Validate(ctx, sessionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Validate after Delete: got %v, want ErrNotFound", err)
	}
}

func TestValidate_ExpiredSimulated(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	store := newTestStore(client)

	sessionID, err := store.Create(ctx, 9)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), fmt.Sprintf("session:%s", sessionID)).Err() })

	// Simulate TTL elapsing deterministically instead of sleeping wall-clock
	// time, per plan.md step 4.
	key := fmt.Sprintf("session:%s", sessionID)
	if err := client.Expire(ctx, key, -1*time.Second).Err(); err != nil {
		t.Fatalf("simulate expiry via Expire: %v", err)
	}

	if _, err := store.Validate(ctx, sessionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Validate after simulated expiry: got %v, want ErrNotFound", err)
	}
}
