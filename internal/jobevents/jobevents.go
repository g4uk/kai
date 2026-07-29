// Package jobevents provides a Redis pub/sub wrapper that fans job-status
// change events out to in-process subscribers, per
// specs/popup-notifications+sse/spec.md and plan.md step 2.
package jobevents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// Channel is the single Redis pub/sub channel used for job status change
// events (spec: "a single Redis pub/sub channel").
const Channel = "job_status_updates"

// StatusChanged is the payload published to Channel whenever a job's status
// changes.
type StatusChanged struct {
	JobID  uint64 `json:"job_id"`
	UserID uint64 `json:"user_id"`
	Status string `json:"status"`
}

// Publish marshals event and publishes it to Channel via client.
func Publish(ctx context.Context, client *redis.Client, event StatusChanged) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("jobevents publish: marshal: %w", err)
	}

	if err := client.Publish(ctx, Channel, raw).Err(); err != nil {
		return fmt.Errorf("jobevents publish: %w", err)
	}

	return nil
}

// subscriberBufferSize is the buffer capacity of each subscriber channel
// registered via Subscribe. dispatch sends non-blockingly, so a subscriber
// that falls behind by more than this many events simply misses the excess
// ones rather than blocking delivery to anyone else (per plan.md step 2 and
// spec edge case 6: a slow/stuck consumer must never block the pipeline).
const subscriberBufferSize = 8

// Broadcaster holds exactly one Redis subscription to Channel (per api
// process, per spec's Constraints) and fans incoming messages out
// in-process to whichever Subscribe'd channels match each message's
// UserID. The zero value is ready to use.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[uint64][]chan []byte
}

// Subscribe registers a new subscriber channel for userID and returns it
// along with an unsubscribe func that removes it from the registry. Callers
// (e.g. the SSE handler, once per open connection) must call unsubscribe
// when done to avoid leaking the channel in the registry.
func (b *Broadcaster) Subscribe(userID uint64) (events <-chan []byte, unsubscribe func()) {
	ch := make(chan []byte, subscriberBufferSize)

	b.mu.Lock()
	if b.subs == nil {
		b.subs = make(map[uint64][]chan []byte)
	}
	b.subs[userID] = append(b.subs[userID], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		chans := b.subs[userID]
		for i, c := range chans {
			if c == ch {
				b.subs[userID] = append(chans[:i], chans[i+1:]...)
				break
			}
		}
		if len(b.subs[userID]) == 0 {
			delete(b.subs, userID)
		}
	}

	return ch, unsub
}

// dispatch decodes raw as a StatusChanged event and non-blockingly delivers
// it to every channel currently registered for that event's UserID, so one
// slow/stuck consumer never blocks delivery to anyone else or the Redis read
// loop in Run. Malformed payloads are silently dropped (there is no
// per-message caller to report an error to).
func (b *Broadcaster) dispatch(raw []byte) {
	var event StatusChanged
	if err := json.Unmarshal(raw, &event); err != nil {
		return
	}

	b.mu.Lock()
	chans := append([]chan []byte(nil), b.subs[event.UserID]...)
	b.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- raw:
		default:
		}
	}
}

// Run subscribes once to Channel and calls dispatch for every incoming
// message until ctx is cancelled, at which point it returns. Per the spec's
// Constraints, an api process should call Run exactly once (not once per
// open SSE connection).
func (b *Broadcaster) Run(ctx context.Context, client *redis.Client) error {
	pubsub := client.Subscribe(ctx, Channel)
	defer func() { _ = pubsub.Close() }()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			b.dispatch([]byte(msg.Payload))
		}
	}
}
