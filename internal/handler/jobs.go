package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/g4uk/kai/internal/job"
)

// videoIDPattern matches a permissive YouTube video ID shape (exact YouTube
// ID rules aren't authoritative here, per plan.md's "Concrete values" note).
var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,}$`)

// youtubeHost and youtuBeHost match the accepted hosts case-insensitively.
var youtubeHost = regexp.MustCompile(`(?i)^(www\.)?youtube\.com$`)
var youtuBeHost = regexp.MustCompile(`(?i)^youtu\.be$`)

// isValidYoutubeURL reports whether raw matches one of the accepted YouTube
// URL shapes: (www.)?youtube.com/watch?v={id} (any additional query params
// tolerated), youtu.be/{id} (optional single trailing slash), or
// (www.)?youtube.com/shorts/{id}.
func isValidYoutubeURL(raw string) bool {
	if raw == "" {
		return false
	}

	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	host := u.Hostname()
	switch {
	case youtubeHost.MatchString(host):
		switch {
		case u.Path == "/watch":
			return videoIDPattern.MatchString(u.Query().Get("v"))
		case strings.HasPrefix(u.Path, "/shorts/"):
			id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/shorts/"), "/")
			return videoIDPattern.MatchString(id)
		}
		return false
	case youtuBeHost.MatchString(host):
		id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/")
		return videoIDPattern.MatchString(id)
	}
	return false
}

// ---- handler-local types (deliberately not internal/job's types — see
// specs/jobs-api/plan.md's isolation note: this mirrors the userFinder
// pattern of keeping internal/handler free of concrete repo types) --------

// Job is the wire representation of an analysis job.
type Job struct {
	ID         uint64    `json:"id"`
	YoutubeURL string    `json:"youtube_url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Participant is the wire representation of a job participant with their
// metrics.
type Participant struct {
	ID      uint64   `json:"id"`
	Label   string   `json:"label"`
	Metrics []Metric `json:"metrics"`
}

// Metric is a single key/value metric for a participant.
type Metric struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

// JobDetail is a Job plus its participants and summary (if any).
type JobDetail struct {
	Job
	Participants []Participant `json:"participants"`
	Summary      *string       `json:"summary"`
}

// ---- consumer-defined interfaces (mirrors the Pinger/OTPRequester pattern) --

// JobCreator creates a new analysis job for userID from youtubeURL.
type JobCreator interface {
	Create(ctx context.Context, userID uint64, youtubeURL string) (Job, error)
}

// JobLister lists all jobs owned by userID, newest-first.
type JobLister interface {
	ListByUser(ctx context.Context, userID uint64) ([]Job, error)
}

// JobGetter fetches a single job (with participants/metrics/summary) owned by
// userID.
type JobGetter interface {
	GetByID(ctx context.Context, id, userID uint64) (JobDetail, error)
}

// ---- request bodies -----------------------------------------------------

type createJobBody struct {
	YoutubeURL string `json:"youtube_url"`
}

// ---- CreateJobHandler ---------------------------------------------------

// CreateJobHandler handles POST /jobs.
type CreateJobHandler struct {
	Jobs JobCreator
}

func (h *CreateJobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body createJobBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !isValidYoutubeURL(body.YoutubeURL) {
		http.Error(w, "invalid youtube_url", http.StatusBadRequest)
		return
	}

	userID, _ := UserIDFromContext(r.Context())

	created, err := h.Jobs.Create(r.Context(), userID, body.YoutubeURL)
	if err != nil {
		if errors.Is(err, job.ErrDuplicate) {
			http.Error(w, "duplicate job", http.StatusConflict)
			return
		}
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

// ---- ListJobsHandler ------------------------------------------------------

// ListJobsHandler handles GET /jobs.
type ListJobsHandler struct {
	Jobs JobLister
}

func (h *ListJobsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	jobs, err := h.Jobs.ListByUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []Job{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Jobs []Job `json:"jobs"`
	}{Jobs: jobs})
}

// ---- GetJobHandler ---------------------------------------------------

// GetJobHandler handles GET /jobs/{id}.
type GetJobHandler struct {
	Jobs JobGetter
}

func (h *GetJobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	userID, _ := UserIDFromContext(r.Context())

	detail, err := h.Jobs.GetByID(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, job.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "get failed", http.StatusInternalServerError)
		return
	}
	if detail.Participants == nil {
		detail.Participants = []Participant{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}
