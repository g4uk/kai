// Package session provides Redis-backed server-side sessions, per
// specs/user-auth/spec.md and specs/user-auth/plan.md step 4.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrNotFound is returned when a session ID does not resolve to a live
// session (unknown, expired, or deleted).
var ErrNotFound = errors.New("session: not found")

// Store implements session creation/validation/deletion against Redis.
type Store struct {
	Client *redis.Client
	TTL    time.Duration
}

func sessionKey(id string) string { return fmt.Sprintf("session:%s", id) }

// Create issues a new cryptographically random session ID bound to userID,
// storing it in Redis with the Store's TTL.
func (s *Store) Create(ctx context.Context, userID uint64) (string, error) {
	id, err := generateID()
	if err != nil {
		return "", fmt.Errorf("session create: %w", err)
	}

	if err := s.Client.Set(ctx, sessionKey(id), userID, s.TTL).Err(); err != nil {
		return "", fmt.Errorf("session create: %w", err)
	}

	return id, nil
}

// Validate resolves a session ID to its userID, returning ErrNotFound if the
// session is missing, expired, or was deleted.
func (s *Store) Validate(ctx context.Context, sessionID string) (uint64, error) {
	val, err := s.Client.Get(ctx, sessionKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("session validate: %w", err)
	}

	userID, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("session validate: parse userID: %w", err)
	}

	return userID, nil
}

// Delete removes a session from Redis.
func (s *Store) Delete(ctx context.Context, sessionID string) error {
	if err := s.Client.Del(ctx, sessionKey(sessionID)).Err(); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}

// generateID returns a cryptographically random 32-byte session token,
// hex-encoded.
func generateID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
