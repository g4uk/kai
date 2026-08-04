// Package jobevents provides a Redis pub/sub wrapper that fans job-status
// change events out to in-process subscribers, per
// specs/popup-notifications+sse/spec.md and plan.md step 2.
package jobevents

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/popup-notifications+sse/plan.md step 2)
//
// internal/jobevents/jobevents.go does not exist yet. This test file
// references the following production identifiers that jobevents.go must
// define; until it does, this package fails to compile (expected, correct
// red state):
//
//	const Channel = "job_status_updates"
//
//	type StatusChanged struct {
//	    JobID  uint64 `json:"job_id"`
//	    UserID uint64 `json:"user_id"`
//	    Status string `json:"status"`
//	}
//
//	func Publish(ctx context.Context, client *redis.Client, event StatusChanged) error
//
//	type Broadcaster struct{ /* unexported registry fields */ }
//	func (b *Broadcaster) Subscribe(userID uint64) (events <-chan []byte, unsubscribe func())
//	func (b *Broadcaster) dispatch(raw []byte)
//	func (b *Broadcaster) Run(ctx context.Context, client *redis.Client) error
//
// dispatch is unexported and tested directly from within this package (no
// real Redis needed) per plan.md's "design split for testability" note: it
// must non-blocking-send (e.g. `select { case ch <- raw: default: }`) to
// every channel registered for the decoded event's UserID, so one
// slow/stuck consumer never blocks another user's delivery or Run's Redis
// read loop. Only TestRun_PublishSubscribeRoundTrip below needs a real
// Redis (TEST_REDIS_ADDR-gated, same convention as internal/otp and
// internal/session).
// ----------------------------------------------------------------------------

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedisClient mirrors internal/session/session_test.go's helper exactly;
// package-local, not shared.
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

func marshalEvent(t *testing.T, event StatusChanged) []byte {
	t.Helper()

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal(%+v): %v", event, err)
	}
	return raw
}

// ---- Broadcaster.dispatch (unit-level, no Redis) -------------------------

func TestBroadcaster_DispatchDeliversToSubscribedUser(t *testing.T) {
	b := &Broadcaster{}
	events, unsubscribe := b.Subscribe(1)
	defer unsubscribe()

	raw := marshalEvent(t, StatusChanged{JobID: 10, UserID: 1, Status: "processing"})
	b.dispatch(raw)

	select {
	case got := <-events:
		if !bytes.Equal(got, raw) {
			t.Errorf("received %s, want %s", got, raw)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("subscribed user's channel did not receive the dispatched event within 1s")
	}
}

func TestBroadcaster_DispatchIsolatesOtherUsers(t *testing.T) {
	b := &Broadcaster{}
	eventsA, unsubA := b.Subscribe(1)
	defer unsubA()
	eventsB, unsubB := b.Subscribe(2)
	defer unsubB()

	raw := marshalEvent(t, StatusChanged{JobID: 10, UserID: 1, Status: "processing"})
	b.dispatch(raw)

	select {
	case got := <-eventsA:
		if !bytes.Equal(got, raw) {
			t.Errorf("user A received %s, want %s", got, raw)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("user A's channel did not receive its own event within 1s")
	}

	select {
	case got := <-eventsB:
		t.Errorf("user B's channel received an event for user A's job: %s, want nothing", got)
	case <-time.After(200 * time.Millisecond):
		// expected: user B's channel gets nothing from a user A event
	}
}

func TestBroadcaster_MultipleSubscribersSameUser(t *testing.T) {
	b := &Broadcaster{}
	first, unsubFirst := b.Subscribe(1)
	defer unsubFirst()
	second, unsubSecond := b.Subscribe(1)
	defer unsubSecond()

	raw := marshalEvent(t, StatusChanged{JobID: 10, UserID: 1, Status: "done"})
	b.dispatch(raw)

	select {
	case got := <-first:
		if !bytes.Equal(got, raw) {
			t.Errorf("first subscriber received %s, want %s", got, raw)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("first subscriber (same user, two open tabs) did not receive the event within 1s")
	}

	select {
	case got := <-second:
		if !bytes.Equal(got, raw) {
			t.Errorf("second subscriber received %s, want %s", got, raw)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("second subscriber (same user, two open tabs) did not receive the event within 1s")
	}
}

func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := &Broadcaster{}
	events, unsubscribe := b.Subscribe(1)

	unsubscribe()

	raw := marshalEvent(t, StatusChanged{JobID: 10, UserID: 1, Status: "processing"})
	b.dispatch(raw)

	select {
	case got, ok := <-events:
		if ok {
			t.Errorf("unsubscribed channel received an event: %s, want nothing", got)
		}
		// A closed channel (ok == false) is also an acceptable way to signal
		// "stopped delivery" — either way, no live event must arrive.
	case <-time.After(200 * time.Millisecond):
		// expected: nothing delivered after unsubscribe
	}
}

func TestBroadcaster_SlowConsumerDoesNotBlock(t *testing.T) {
	b := &Broadcaster{}
	_, unsubSlow := b.Subscribe(1)
	defer unsubSlow()
	other, unsubOther := b.Subscribe(2)
	defer unsubOther()

	rawSlow := marshalEvent(t, StatusChanged{JobID: 10, UserID: 1, Status: "processing"})

	// Fill user 1's subscriber channel well past any reasonable buffer
	// capacity, without ever reading from `slow`. dispatch must be
	// non-blocking (plan.md step 2: `select { case ch <- raw: default: }`);
	// if the implementation instead used a blocking send, this loop would
	// hang forever. Run it in a goroutine guarded by a timeout so a bug here
	// fails this test instead of hanging the whole suite.
	fillDone := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.dispatch(rawSlow)
		}
		close(fillDone)
	}()

	select {
	case <-fillDone:
	case <-time.After(1 * time.Second):
		t.Fatal("dispatch() to a full/unread subscriber channel did not return promptly; want a non-blocking send")
	}

	rawOther := marshalEvent(t, StatusChanged{JobID: 11, UserID: 2, Status: "processing"})
	b.dispatch(rawOther)

	select {
	case got := <-other:
		if !bytes.Equal(got, rawOther) {
			t.Errorf("other user's channel received %s, want %s", got, rawOther)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("a different user's channel did not receive its event promptly after another user's channel was filled")
	}
}

// ---- Run / Publish round trip (TEST_REDIS_ADDR-gated) --------------------

func TestRun_PublishSubscribeRoundTrip(t *testing.T) {
	client := testRedisClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := &Broadcaster{}
	runDone := make(chan error, 1)
	go func() { runDone <- b.Run(ctx, client) }()

	// Redis pub/sub is fire-and-forget: give Run a moment to establish its
	// subscription before publishing, matching the spec's own Non-scope
	// note that there's no backfill/replay for messages published before a
	// subscriber exists.
	time.Sleep(200 * time.Millisecond)

	const wantUserID = uint64(7)
	events, unsubscribe := b.Subscribe(wantUserID)
	defer unsubscribe()

	want := StatusChanged{JobID: 42, UserID: wantUserID, Status: "processing", Type: "status"}
	if err := Publish(ctx, client, want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case raw := <-events:
		var got StatusChanged
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", raw, err)
		}
		if got != want {
			t.Errorf("received event %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("published event did not arrive via Run/Subscribe within 2s")
	}

	cancel()
	select {
	case <-runDone:
		// Run returned after ctx cancellation — expected, regardless of the
		// specific error value (e.g. context.Canceled) it reports.
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return within 1s after context cancellation")
	}
}

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/video-processing-improvements/plan.md step 5)
//
// The tests below reference a new event shape and publish function that
// don't exist yet, so this package fails to compile until jobevents.go
// defines:
//
//	type StageChanged struct {
//	    JobID  uint64 `json:"job_id"`
//	    UserID uint64 `json:"user_id"`
//	    Stage  string `json:"stage"`
//	    Type   string `json:"type"`
//	}
//
//	func PublishStage(ctx context.Context, client *redis.Client, event StageChanged) error
//
// StatusChanged also gains a `Type string` field (set to "status" by
// Publish; PublishStage sets StageChanged's Type to "stage") --
// internal/handler/jobs_stream.go (step 11/12) needs this discriminator to
// pick the right SSE event name. Broadcaster.dispatch is unchanged: it
// already forwards raw bytes and only needs UserID to route, which both
// event shapes carry.
// ----------------------------------------------------------------------------

func marshalStageEvent(t *testing.T, event StageChanged) []byte {
	t.Helper()

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal(%+v): %v", event, err)
	}
	return raw
}

// TestBroadcaster_DispatchRoutesStageChangedPayload covers plan.md step 5b:
// a StageChanged-shaped payload (no "status" field) is still routed to the
// correct UserID's subscriber, with dispatch forwarding the raw bytes
// unmodified — exactly like it already does for StatusChanged.
func TestBroadcaster_DispatchRoutesStageChangedPayload(t *testing.T) {
	b := &Broadcaster{}
	events, unsubscribe := b.Subscribe(1)
	defer unsubscribe()

	raw := marshalStageEvent(t, StageChanged{JobID: 10, UserID: 1, Stage: "downloading", Type: "stage"})
	b.dispatch(raw)

	select {
	case got := <-events:
		if !bytes.Equal(got, raw) {
			t.Errorf("received %s, want %s (dispatch must forward raw bytes unmodified)", got, raw)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("subscribed user's channel did not receive the dispatched StageChanged event within 1s")
	}
}

// TestStatusChangedAndStageChanged_MarshalIncludesTypeField covers plan.md
// step 5c: both event shapes' JSON now carry a "type" discriminator field
// (internal/handler/jobs_stream.go's step 11/12 needs it to pick the right
// SSE event name), populated with their shape-specific value.
func TestStatusChangedAndStageChanged_MarshalIncludesTypeField(t *testing.T) {
	statusRaw := marshalEvent(t, StatusChanged{JobID: 1, UserID: 2, Status: "processing", Type: "status"})
	var statusDecoded map[string]any
	if err := json.Unmarshal(statusRaw, &statusDecoded); err != nil {
		t.Fatalf("json.Unmarshal(StatusChanged): %v", err)
	}
	if got := statusDecoded["type"]; got != "status" {
		t.Errorf(`StatusChanged JSON "type" = %v, want "status"`, got)
	}

	stageRaw := marshalStageEvent(t, StageChanged{JobID: 1, UserID: 2, Stage: "probing", Type: "stage"})
	var stageDecoded map[string]any
	if err := json.Unmarshal(stageRaw, &stageDecoded); err != nil {
		t.Fatalf("json.Unmarshal(StageChanged): %v", err)
	}
	if got := stageDecoded["type"]; got != "stage" {
		t.Errorf(`StageChanged JSON "type" = %v, want "stage"`, got)
	}
}

// TestPublishStage_MarshalsAndPublishes covers plan.md step 5a
// (TEST_REDIS_ADDR-gated, same convention as TestRun_PublishSubscribeRoundTrip):
// a StageChanged event round-trips through Redis via PublishStage, and the
// received payload carries Type "stage" -- proving PublishStage (like
// Publish for StatusChanged) populates the type discriminator itself, not
// just that the field exists.
func TestPublishStage_MarshalsAndPublishes(t *testing.T) {
	client := testRedisClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := &Broadcaster{}
	runDone := make(chan error, 1)
	go func() { runDone <- b.Run(ctx, client) }()

	time.Sleep(200 * time.Millisecond)

	const wantUserID = uint64(9)
	events, unsubscribe := b.Subscribe(wantUserID)
	defer unsubscribe()

	want := StageChanged{JobID: 43, UserID: wantUserID, Stage: "analyzing"}
	if err := PublishStage(ctx, client, want); err != nil {
		t.Fatalf("PublishStage: %v", err)
	}

	select {
	case raw := <-events:
		var got StageChanged
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", raw, err)
		}
		if got.JobID != want.JobID || got.UserID != want.UserID || got.Stage != want.Stage {
			t.Errorf("received event %+v, want JobID/UserID/Stage matching %+v", got, want)
		}
		if got.Type != "stage" {
			t.Errorf("received event Type = %q, want %q", got.Type, "stage")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("published StageChanged event did not arrive via Run/Subscribe within 2s")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return within 1s after context cancellation")
	}
}
