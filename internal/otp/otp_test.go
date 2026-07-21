package otp

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// This test file drives internal/otp/otp.go, which does not exist yet
// (specs/user-auth/plan.md step 3). Expect a compile error until otp.go
// defines:
//
//	type Service struct {
//	    Client        *redis.Client
//	    CodeTTL       time.Duration
//	    RequestWindow time.Duration
//	    MaxRequests   int
//	    MaxAttempts   int
//	}
//	func (s *Service) Request(ctx context.Context, phone string) (string, error)
//	func (s *Service) Verify(ctx context.Context, phone, code string) error
//	var ErrNotFound, ErrExpired, ErrMismatch, ErrTooManyAttempts, ErrRateLimited error
//
// ASSUMPTION (documented, to be reconciled by the implementer): plan.md step 5
// lists the handler-local interface as `OTPRequester.Request(ctx, phone) error`
// (no returned code), but step 3's own narrative says Request "returns the
// plaintext code to the caller for the stub log line — logging itself happens
// in the handler, not this package." Only the handler can log the code if
// Request returns it, so this test assumes the concrete Service method is
// `Request(ctx, phone string) (string, error)`; the handler-local OTPRequester
// interface (internal/handler/auth_test.go) mirrors this signature.
//
// Redis key layout under test (per plan.md): otp:{phone}:code,
// otp:{phone}:attempts, otp:{phone}:reqcount.

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping integration test")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	return client
}

// uniquePhone gives each test its own phone number namespace so tests don't
// interfere with each other's OTP/attempts/reqcount keys.
func uniquePhone(t *testing.T) string {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	return fmt.Sprintf("+1%09d", h.Sum32()%1_000_000_000)
}

func newTestService(client *redis.Client) *Service {
	return &Service{
		Client:        client,
		CodeTTL:       5 * time.Minute,
		RequestWindow: time.Hour,
		MaxRequests:   5,
		MaxAttempts:   5,
	}
}

func cleanupOTPKeys(t *testing.T, client *redis.Client, phone string) {
	t.Helper()
	t.Cleanup(func() {
		_ = client.Del(context.Background(),
			fmt.Sprintf("otp:%s:code", phone),
			fmt.Sprintf("otp:%s:attempts", phone),
			fmt.Sprintf("otp:%s:reqcount", phone),
		).Err()
	})
}

func TestRequestVerify_Success(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	code, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("Request returned code %q, want a 6-digit code", code)
	}

	if err := svc.Verify(ctx, phone, code); err != nil {
		t.Fatalf("Verify with correct code: %v", err)
	}
}

func TestVerify_WrongCode_IncrementsAttempts(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	if _, err := svc.Request(ctx, phone); err != nil {
		t.Fatalf("Request: %v", err)
	}

	if err := svc.Verify(ctx, phone, "000000"); !errors.Is(err, ErrMismatch) {
		t.Fatalf("Verify wrong code: got %v, want ErrMismatch", err)
	}

	attemptsKey := fmt.Sprintf("otp:%s:attempts", phone)
	got, err := client.Get(ctx, attemptsKey).Result()
	if err != nil {
		t.Fatalf("get attempts key: %v", err)
	}
	if got != "1" {
		t.Errorf("attempts counter = %q, want %q after one wrong attempt", got, "1")
	}
}

func TestVerify_TooManyAttempts_InvalidatesCode(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	code, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	// MaxAttempts is 5: the first 5 wrong guesses are plain mismatches.
	for i := 0; i < 5; i++ {
		if err := svc.Verify(ctx, phone, "000000"); !errors.Is(err, ErrMismatch) {
			t.Fatalf("wrong attempt %d: got %v, want ErrMismatch", i+1, err)
		}
	}

	// The 6th wrong attempt exceeds the limit and invalidates the code.
	if err := svc.Verify(ctx, phone, "000000"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("6th wrong attempt: got %v, want ErrTooManyAttempts", err)
	}

	// The code is now invalidated, so even the correct code fails.
	if err := svc.Verify(ctx, phone, code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Verify correct code after too-many-attempts: got %v, want ErrNotFound", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	code, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	// Simulate TTL elapsing deterministically instead of sleeping wall-clock
	// time, per plan.md step 3.
	codeKey := fmt.Sprintf("otp:%s:code", phone)
	if err := client.Expire(ctx, codeKey, -1*time.Second).Err(); err != nil {
		t.Fatalf("simulate expiry via Expire: %v", err)
	}

	if err := svc.Verify(ctx, phone, code); !errors.Is(err, ErrExpired) {
		t.Fatalf("Verify after simulated expiry: got %v, want ErrExpired", err)
	}
}

func TestVerify_Replay_AlreadyConsumed(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	code, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if err := svc.Verify(ctx, phone, code); err != nil {
		t.Fatalf("first Verify: %v", err)
	}

	if err := svc.Verify(ctx, phone, code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replayed Verify of consumed code: got %v, want ErrNotFound", err)
	}
}

func TestRequest_RateLimited(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	// MaxRequests is 5: the first 5 requests within the window succeed.
	for i := 0; i < 5; i++ {
		if _, err := svc.Request(ctx, phone); err != nil {
			t.Fatalf("request %d: got %v, want nil", i+1, err)
		}
	}

	// The 6th request within the same window is rejected.
	if _, err := svc.Request(ctx, phone); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th Request in window: got %v, want ErrRateLimited", err)
	}
}

func TestRequest_InvalidatesPriorCode(t *testing.T) {
	client := testRedisClient(t)
	ctx := context.Background()
	phone := uniquePhone(t)
	cleanupOTPKeys(t, client, phone)
	svc := newTestService(client)

	firstCode, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("first Request: %v", err)
	}

	secondCode, err := svc.Request(ctx, phone)
	if err != nil {
		t.Fatalf("second Request: %v", err)
	}
	if secondCode == firstCode {
		t.Fatal("second Request produced the same code as the first; codes must differ to test invalidation")
	}

	// The prior code must no longer verify; we don't pin the exact sentinel
	// here since plan.md doesn't specify one for this case (only that "the
	// prior OTP is invalidated"), just that it must fail.
	if err := svc.Verify(ctx, phone, firstCode); err == nil {
		t.Fatal("Verify with prior (invalidated) code: want non-nil error, got nil")
	}

	if err := svc.Verify(ctx, phone, secondCode); err != nil {
		t.Fatalf("Verify with newest code: got %v, want nil", err)
	}
}
