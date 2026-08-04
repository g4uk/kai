package handler

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/popup-notifications+sse/plan.md step 3)
//
// internal/handler/jobs_stream.go does not exist yet. This test file
// references the following production identifiers that jobs_stream.go must
// define; until it does, this package fails to compile (expected, correct
// red state):
//
//	// JobEventSubscriber mirrors the Pinger/SessionValidator consumer-defined
//	// interface pattern; satisfied by *jobevents.Broadcaster.
//	type JobEventSubscriber interface {
//	    Subscribe(userID uint64) (events <-chan []byte, unsubscribe func())
//	}
//
//	type JobStreamHandler struct {
//	    Events             JobEventSubscriber
//	    Sessions           SessionValidator // reused from auth.go, not redeclared
//	    RevalidateInterval time.Duration    // defaults to 60s in production wiring
//	}
//	func (h *JobStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
//
// ServeHTTP reads userID via UserIDFromContext (already validated once by
// SessionMiddleware wrapping this route), writes SSE headers, flushes 200,
// Subscribe()s, and loops on select across r.Context().Done() (return),
// the event channel (write "event: job_status\ndata: <json>\n\n", flush),
// and a RevalidateInterval ticker (re-validate the session cookie via
// h.Sessions.Validate; return if it errors) — per plan.md step 3.
//
// TEST APPROACH NOTE (no precedent elsewhere in this repo, per plan.md's
// Risks section): ServeHTTP loops forever until its request context is
// cancelled, so each test below runs it in a goroutine against a
// context.WithCancel-backed request, drives the scenario, cancels the
// context, and joins on a "done" channel (closed when ServeHTTP returns)
// guarded by a time.After(1s) timeout so a bug can't hang the suite. Per
// that same Risks note, tests that need to observe body content
// (TestJobStreamHandler_DeliversEvent) use an UNBUFFERED stub subscriber
// channel: sending on it blocks until ServeHTTP's select has received the
// value, giving the test a synchronization point *before* it cancels the
// context — so rec.Body is only ever read by the test goroutine after
// joining "done" (i.e. after ServeHTTP has returned and stopped writing to
// it), avoiding a data race under `go test -race`.
// ----------------------------------------------------------------------------

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g4uk/kai/internal/session"
)

// ---- stubs ------------------------------------------------------------

// stubEventSubscriber is a local JobEventSubscriber whose Subscribe returns a
// test-controlled channel and tracks whether unsubscribe was called.
type stubEventSubscriber struct {
	ch            chan []byte
	unsubscribedP *bool
}

func (s stubEventSubscriber) Subscribe(_ uint64) (<-chan []byte, func()) {
	return s.ch, func() {
		if s.unsubscribedP != nil {
			*s.unsubscribedP = true
		}
	}
}

// ---- JobStreamHandler ---------------------------------------------------

func TestJobStreamHandler_Unauthenticated401(t *testing.T) {
	// Mirrors auth_test.go's TestSessionMiddleware "missing session cookie"
	// case: SessionMiddleware rejects the request with 401 before
	// JobStreamHandler.ServeHTTP is ever reached, so no goroutine/cancel
	// dance is needed here (per spec criterion 10).
	h := SessionMiddleware(&JobStreamHandler{}, stubSessionValidator{userID: testUserID})

	req := httptest.NewRequest(http.MethodGet, "/jobs/stream", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestJobStreamHandler_DeliversEvent(t *testing.T) {
	// Unbuffered: sending blocks until ServeHTTP's select receives it (see
	// package-level TEST APPROACH NOTE above).
	events := make(chan []byte)
	sub := stubEventSubscriber{ch: events}
	h := &JobStreamHandler{
		Events:             sub,
		Sessions:           stubSessionValidator{userID: testUserID},
		RevalidateInterval: time.Hour, // long enough to never fire during this test
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/jobs/stream", nil)
	req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
	req = req.WithContext(ctx)
	req = withUserID(req, testUserID)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	raw := []byte(`{"job_id":1,"status":"processing"}`)
	select {
	case events <- raw:
		// ServeHTTP's select received the value; it will now write it to the
		// response before it can loop back and observe ctx cancellation.
	case <-time.After(1 * time.Second):
		t.Fatal("handler never read from its event channel within 1s")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("ServeHTTP did not return within 1s after context cancellation")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/event-stream")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: job_status") {
		t.Errorf("body = %q, want it to contain %q", body, "event: job_status")
	}
	if !strings.Contains(body, string(raw)) {
		t.Errorf("body = %q, want it to contain the event payload %s", body, raw)
	}
}

func TestJobStreamHandler_ClosesOnClientDisconnect(t *testing.T) {
	unsubscribed := false
	sub := stubEventSubscriber{ch: make(chan []byte), unsubscribedP: &unsubscribed}
	h := &JobStreamHandler{
		Events:             sub,
		Sessions:           stubSessionValidator{userID: testUserID},
		RevalidateInterval: time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/jobs/stream", nil)
	req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
	req = req.WithContext(ctx)
	req = withUserID(req, testUserID)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("ServeHTTP did not return within 1s after client disconnect (context cancellation)")
	}

	if !unsubscribed {
		t.Error("expected the subscriber's unsubscribe func to be called on client disconnect")
	}
}

func TestJobStreamHandler_ClosesOnRevalidationFailure(t *testing.T) {
	sub := stubEventSubscriber{ch: make(chan []byte)}
	h := &JobStreamHandler{
		Events:             sub,
		Sessions:           stubSessionValidator{err: session.ErrNotFound},
		RevalidateInterval: 5 * time.Millisecond, // real 60s would make this test far too slow
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/jobs/stream", nil)
	req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
	req = req.WithContext(ctx)
	req = withUserID(req, testUserID)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		// handler closed the stream on its own once revalidation failed —
		// expected, per spec criterion 7.
	case <-time.After(1 * time.Second):
		t.Fatal("ServeHTTP did not return within 1s after a revalidation failure (RevalidateInterval=5ms); want it to close the stream promptly, not wait for a real 60s interval")
	}
}

// TestJobStreamHandler_EventNameDependsOnPayloadType covers
// specs/video-processing-improvements/plan.md step 11: ServeHTTP picks the
// SSE event name from the payload's "type" field — "status" writes
// "event: job_status", "stage" writes "event: job_stage", and a payload
// with no "type" field (an old/malformed payload) defensively defaults to
// "event: job_status" so it's never silently dropped or crashes the stream.
func TestJobStreamHandler_EventNameDependsOnPayloadType(t *testing.T) {
	cases := []struct {
		name          string
		payload       string
		wantEventName string
	}{
		{
			name:          `type "status" writes event: job_status`,
			payload:       `{"type":"status","job_id":1,"user_id":1,"status":"processing"}`,
			wantEventName: "event: job_status",
		},
		{
			name:          `type "stage" writes event: job_stage`,
			payload:       `{"type":"stage","job_id":1,"user_id":1,"stage":"downloading"}`,
			wantEventName: "event: job_stage",
		},
		{
			name:          "missing type field defaults to event: job_status",
			payload:       `{"job_id":1,"user_id":1,"status":"processing"}`,
			wantEventName: "event: job_status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Unbuffered, see package TEST APPROACH NOTE above.
			events := make(chan []byte)
			sub := stubEventSubscriber{ch: events}
			h := &JobStreamHandler{
				Events:             sub,
				Sessions:           stubSessionValidator{userID: testUserID},
				RevalidateInterval: time.Hour,
			}

			ctx, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest(http.MethodGet, "/jobs/stream", nil)
			req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
			req = req.WithContext(ctx)
			req = withUserID(req, testUserID)
			rec := httptest.NewRecorder()

			done := make(chan struct{})
			go func() {
				h.ServeHTTP(rec, req)
				close(done)
			}()

			raw := []byte(tc.payload)
			select {
			case events <- raw:
				// ServeHTTP's select received the value; see package TEST
				// APPROACH NOTE for why this ordering avoids a data race.
			case <-time.After(1 * time.Second):
				t.Fatal("handler never read from its event channel within 1s")
			}

			cancel()

			select {
			case <-done:
			case <-time.After(1 * time.Second):
				t.Fatal("ServeHTTP did not return within 1s after context cancellation")
			}

			body := rec.Body.String()
			if !strings.Contains(body, tc.wantEventName) {
				t.Errorf("body = %q, want it to contain %q", body, tc.wantEventName)
			}
			if !strings.Contains(body, string(raw)) {
				t.Errorf("body = %q, want it to contain the event payload %s", body, raw)
			}
		})
	}
}
