package handler

import (
	"fmt"
	"net/http"
	"time"
)

// JobEventSubscriber is a consumer-defined interface (mirrors the
// Pinger/SessionValidator pattern) satisfied by *jobevents.Broadcaster,
// without importing that concrete type into this package.
type JobEventSubscriber interface {
	Subscribe(userID uint64) (events <-chan []byte, unsubscribe func())
}

// JobStreamHandler handles GET /jobs/stream, an SSE endpoint that streams
// the authenticated user's own job status-change events, per
// specs/popup-notifications+sse/spec.md. It must be wrapped in
// SessionMiddleware, same as every other /jobs* route.
type JobStreamHandler struct {
	Events   JobEventSubscriber
	Sessions SessionValidator

	// RevalidateInterval controls how often the open connection's session is
	// re-validated (spec: every 60s in production; tests use a much shorter
	// interval so they don't have to wait a real minute).
	RevalidateInterval time.Duration
}

func (h *JobStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, unsubscribe := h.Events.Subscribe(userID)
	defer unsubscribe()

	ticker := time.NewTicker(h.RevalidateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case raw := <-events:
			if _, err := fmt.Fprintf(w, "event: job_status\ndata: %s\n\n", raw); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := h.Sessions.Validate(r.Context(), cookie.Value); err != nil {
				return
			}
		}
	}
}
