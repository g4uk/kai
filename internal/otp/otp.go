// Package otp implements the one-time-passcode lifecycle (request, verify,
// rate limiting) backed by Redis, per specs/user-auth/spec.md and
// specs/user-auth/plan.md step 3.
package otp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sentinel errors returned by Verify/Request.
var (
	ErrNotFound        = errors.New("otp: not found")
	ErrExpired         = errors.New("otp: expired")
	ErrMismatch        = errors.New("otp: code mismatch")
	ErrTooManyAttempts = errors.New("otp: too many attempts")
	ErrRateLimited     = errors.New("otp: rate limited")
)

// Service implements the OTP lifecycle against Redis. All durations/limits
// are injected (not hardcoded) so tests can use short-but-nonzero windows.
type Service struct {
	Client        *redis.Client
	CodeTTL       time.Duration
	RequestWindow time.Duration
	MaxRequests   int
	MaxAttempts   int
}

func codeKey(phone string) string     { return fmt.Sprintf("otp:%s:code", phone) }
func attemptsKey(phone string) string { return fmt.Sprintf("otp:%s:attempts", phone) }
func reqCountKey(phone string) string { return fmt.Sprintf("otp:%s:reqcount", phone) }
func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// Request generates a new 6-digit numeric code, stores its salted hash (plain
// sha256 of the code; no separate salt needed since codes are short-lived,
// single-use, and rate-limited) in Redis with CodeTTL, resets the attempts
// counter, invalidates any prior live code, and enforces MaxRequests per
// RequestWindow via an incrementing counter key. It returns the plaintext
// code to the caller so the handler can log it (stub for SMS delivery);
// logging itself does not happen in this package.
func (s *Service) Request(ctx context.Context, phone string) (string, error) {
	rcKey := reqCountKey(phone)
	count, err := s.Client.Incr(ctx, rcKey).Result()
	if err != nil {
		return "", fmt.Errorf("otp request: incr reqcount: %w", err)
	}
	if count == 1 {
		if err := s.Client.Expire(ctx, rcKey, s.RequestWindow).Err(); err != nil {
			return "", fmt.Errorf("otp request: expire reqcount: %w", err)
		}
	}
	if int(count) > s.MaxRequests {
		return "", ErrRateLimited
	}

	code, err := generateCode()
	if err != nil {
		return "", fmt.Errorf("otp request: generate code: %w", err)
	}

	pipe := s.Client.TxPipeline()
	pipe.Set(ctx, codeKey(phone), hashCode(code), s.CodeTTL)
	// attemptsKey shares codeKey's TTL and acts as a liveness marker: it lets
	// Verify distinguish "code naturally expired" (attemptsKey still present,
	// codeKey gone) from "never requested / already consumed" (both gone),
	// since Redis deletes an expired code key outright rather than leaving a
	// tombstone behind.
	pipe.Set(ctx, attemptsKey(phone), "0", s.CodeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("otp request: store code: %w", err)
	}

	return code, nil
}

// Verify checks the given code against the stored hash for phone. On
// success the code is deleted (single-use). On mismatch the attempts
// counter is incremented; hitting MaxAttempts invalidates the code early.
func (s *Service) Verify(ctx context.Context, phone, code string) error {
	cKey := codeKey(phone)
	aKey := attemptsKey(phone)

	stored, err := s.Client.Get(ctx, cKey).Result()
	if errors.Is(err, redis.Nil) {
		// codeKey is gone. If the attempts marker (set with the same TTL at
		// Request time) is still present, the code must have expired rather
		// than never having existed or already been consumed/invalidated.
		exists, existsErr := s.Client.Exists(ctx, aKey).Result()
		if existsErr != nil {
			return fmt.Errorf("otp verify: check attempts marker: %w", existsErr)
		}
		if exists > 0 {
			return ErrExpired
		}
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("otp verify: get code: %w", err)
	}

	if stored != hashCode(code) {
		attempts, err := s.Client.Incr(ctx, aKey).Result()
		if err != nil {
			return fmt.Errorf("otp verify: incr attempts: %w", err)
		}
		if int(attempts) > s.MaxAttempts {
			_ = s.Client.Del(ctx, cKey, aKey).Err()
			return ErrTooManyAttempts
		}
		return ErrMismatch
	}

	if err := s.Client.Del(ctx, cKey, aKey).Err(); err != nil {
		return fmt.Errorf("otp verify: delete code: %w", err)
	}

	return nil
}

// generateCode returns a cryptographically random 6-digit numeric code,
// zero-padded.
func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
