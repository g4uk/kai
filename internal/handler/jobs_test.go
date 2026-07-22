package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g4uk/kai/internal/job"
)

// errUnexpected stands in for an opaque/unexpected repository error (e.g. a
// dropped DB connection) that isn't one of the handler's known sentinel
// cases, exercising the generic-error-maps-to-500 path.
var errUnexpected = errors.New("unexpected repository error")

// ---- stubs ------------------------------------------------------------

type stubJobCreator struct {
	job     Job
	err     error
	calledP *bool
}

func (s stubJobCreator) Create(_ context.Context, _ uint64, _ string) (Job, error) {
	if s.calledP != nil {
		*s.calledP = true
	}
	return s.job, s.err
}

type stubJobLister struct {
	jobs []Job
	err  error
}

func (s stubJobLister) ListByUser(_ context.Context, _ uint64) ([]Job, error) {
	return s.jobs, s.err
}

type stubJobGetter struct {
	detail  JobDetail
	err     error
	calledP *bool
}

func (s stubJobGetter) GetByID(_ context.Context, _, _ uint64) (JobDetail, error) {
	if s.calledP != nil {
		*s.calledP = true
	}
	return s.detail, s.err
}

// ---- test helpers -------------------------------------------------------

// withUserID attaches userID to the request context the same way
// SessionMiddleware does (via the unexported userIDContextKey already defined
// in auth.go), so handler tests can call ServeHTTP directly without going
// through the middleware.
func withUserID(r *http.Request, userID uint64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDContextKey, userID))
}

const testUserID = uint64(42)

// ---- CreateJobHandler ---------------------------------------------------

func TestCreateJobHandler(t *testing.T) {
	fixedTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	validURL := "https://www.youtube.com/watch?v=abc123def45"

	tests := []struct {
		name       string
		body       string
		creator    stubJobCreator
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "empty youtube_url",
			body:       `{"youtube_url":""}`,
			creator:    stubJobCreator{job: Job{ID: 1, YoutubeURL: validURL, Status: "pending"}},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "missing youtube_url field",
			body:       `{}`,
			creator:    stubJobCreator{job: Job{ID: 1, YoutubeURL: validURL, Status: "pending"}},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "malformed (non-YouTube) youtube_url",
			body:       `{"youtube_url":"https://example.com/not-youtube"}`,
			creator:    stubJobCreator{job: Job{ID: 1, YoutubeURL: validURL, Status: "pending"}},
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name: "valid youtube_url creates job",
			body: `{"youtube_url":"` + validURL + `"}`,
			creator: stubJobCreator{job: Job{
				ID:         7,
				YoutubeURL: validURL,
				Status:     "pending",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			}},
			wantStatus: http.StatusCreated,
			wantCalled: true,
		},
		{
			name:       "duplicate non-failed job for user+URL",
			body:       `{"youtube_url":"` + validURL + `"}`,
			creator:    stubJobCreator{err: job.ErrDuplicate},
			wantStatus: http.StatusConflict,
			wantCalled: true,
		},
		{
			name:       "unexpected repository error",
			body:       `{"youtube_url":"` + validURL + `"}`,
			creator:    stubJobCreator{err: errUnexpected},
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			tt.creator.calledP = &called
			h := &CreateJobHandler{Jobs: tt.creator}

			req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(tt.body))
			req = withUserID(req, testUserID)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d (body=%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if called != tt.wantCalled {
				t.Errorf("Jobs.Create called = %v, want %v", called, tt.wantCalled)
			}

			if tt.wantStatus == http.StatusCreated {
				var got Job
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
				}
				if got.ID != tt.creator.job.ID {
					t.Errorf("response.ID = %d, want %d", got.ID, tt.creator.job.ID)
				}
				if got.YoutubeURL != tt.creator.job.YoutubeURL {
					t.Errorf("response.YoutubeURL = %q, want %q", got.YoutubeURL, tt.creator.job.YoutubeURL)
				}
				if got.Status != tt.creator.job.Status {
					t.Errorf("response.Status = %q, want %q", got.Status, tt.creator.job.Status)
				}
				if !got.CreatedAt.Equal(tt.creator.job.CreatedAt) {
					t.Errorf("response.CreatedAt = %v, want %v", got.CreatedAt, tt.creator.job.CreatedAt)
				}
				if !got.UpdatedAt.Equal(tt.creator.job.UpdatedAt) {
					t.Errorf("response.UpdatedAt = %v, want %v", got.UpdatedAt, tt.creator.job.UpdatedAt)
				}
			}
		})
	}
}

// ---- ListJobsHandler ------------------------------------------------------

func TestListJobsHandler(t *testing.T) {
	t.Run("three jobs returns exactly three entries", func(t *testing.T) {
		lister := stubJobLister{jobs: []Job{
			{ID: 1, YoutubeURL: "https://youtu.be/aaaaaaaaaaa", Status: "pending"},
			{ID: 2, YoutubeURL: "https://youtu.be/bbbbbbbbbbb", Status: "done"},
			{ID: 3, YoutubeURL: "https://youtu.be/ccccccccccc", Status: "failed"},
		}}
		h := &ListJobsHandler{Jobs: lister}

		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		req = withUserID(req, testUserID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var got struct {
			Jobs []Job `json:"jobs"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
		}
		if len(got.Jobs) != 3 {
			t.Errorf("len(jobs) = %d, want 3", len(got.Jobs))
		}
	})

	t.Run("zero jobs returns empty array, never null", func(t *testing.T) {
		lister := stubJobLister{jobs: []Job{}}
		h := &ListJobsHandler{Jobs: lister}

		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		req = withUserID(req, testUserID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"jobs":[]`) {
			t.Errorf("response body = %q, want it to contain literal %q", body, `"jobs":[]`)
		}
		if strings.Contains(body, "null") {
			t.Errorf("response body = %q, must not contain %q", body, "null")
		}
	})
}

// ---- GetJobHandler ---------------------------------------------------

// dispatchGetJob routes req through a small local ServeMux registering the
// same "GET /jobs/{id}" pattern the real server uses, so h can read
// r.PathValue("id") exactly as it will in production (per cmd/api/main.go's
// Go 1.22+ pattern-based routing).
func dispatchGetJob(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET /jobs/{id}", h)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestGetJobHandler(t *testing.T) {
	summary := "close bout, 4-3 on point differential"

	t.Run("full detail with participants, metrics and summary", func(t *testing.T) {
		detail := JobDetail{
			Job: Job{
				ID:         5,
				YoutubeURL: "https://youtu.be/aaaaaaaaaaa",
				Status:     "done",
			},
			Participants: []Participant{
				{ID: 1, Label: "Alice", Metrics: []Metric{{Key: "strikes", Value: 12.5}, {Key: "speed", Value: 3.2}}},
				{ID: 2, Label: "Bob", Metrics: []Metric{{Key: "strikes", Value: 9}}},
			},
			Summary: &summary,
		}
		getter := stubJobGetter{detail: detail}
		h := &GetJobHandler{Jobs: getter}

		req := httptest.NewRequest(http.MethodGet, "/jobs/5", nil)
		req = withUserID(req, testUserID)
		rec := dispatchGetJob(h, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var got JobDetail
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
		}
		if got.ID != detail.ID {
			t.Errorf("response.ID = %d, want %d", got.ID, detail.ID)
		}
		if len(got.Participants) != 2 {
			t.Fatalf("len(Participants) = %d, want 2", len(got.Participants))
		}
		if got.Participants[0].Label != "Alice" || len(got.Participants[0].Metrics) != 2 {
			t.Errorf("Participants[0] = %+v, want Alice with 2 metrics", got.Participants[0])
		}
		if got.Participants[1].Label != "Bob" || len(got.Participants[1].Metrics) != 1 {
			t.Errorf("Participants[1] = %+v, want Bob with 1 metric", got.Participants[1])
		}
		if got.Summary == nil || *got.Summary != summary {
			t.Errorf("Summary = %v, want %q", got.Summary, summary)
		}
	})

	t.Run("empty participants serializes as empty array, never null", func(t *testing.T) {
		detail := JobDetail{
			Job:          Job{ID: 6, YoutubeURL: "https://youtu.be/bbbbbbbbbbb", Status: "pending"},
			Participants: []Participant{},
		}
		getter := stubJobGetter{detail: detail}
		h := &GetJobHandler{Jobs: getter}

		req := httptest.NewRequest(http.MethodGet, "/jobs/6", nil)
		req = withUserID(req, testUserID)
		rec := dispatchGetJob(h, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"participants":[]`) {
			t.Errorf("response body = %q, want it to contain literal %q", rec.Body.String(), `"participants":[]`)
		}
	})

	t.Run("nil summary serializes as null", func(t *testing.T) {
		detail := JobDetail{
			Job:          Job{ID: 7, YoutubeURL: "https://youtu.be/ccccccccccc", Status: "pending"},
			Participants: []Participant{},
			Summary:      nil,
		}
		getter := stubJobGetter{detail: detail}
		h := &GetJobHandler{Jobs: getter}

		req := httptest.NewRequest(http.MethodGet, "/jobs/7", nil)
		req = withUserID(req, testUserID)
		rec := dispatchGetJob(h, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"summary":null`) {
			t.Errorf("response body = %q, want it to contain literal %q", rec.Body.String(), `"summary":null`)
		}
	})

	t.Run("not found sentinel maps to 404", func(t *testing.T) {
		getter := stubJobGetter{err: job.ErrNotFound}
		h := &GetJobHandler{Jobs: getter}

		req := httptest.NewRequest(http.MethodGet, "/jobs/999", nil)
		req = withUserID(req, testUserID)
		rec := dispatchGetJob(h, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status: got %d, want %d (body=%s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})

	t.Run("non-numeric id is rejected before the store is called", func(t *testing.T) {
		called := false
		getter := stubJobGetter{calledP: &called}
		h := &GetJobHandler{Jobs: getter}

		req := httptest.NewRequest(http.MethodGet, "/jobs/abc", nil)
		req = withUserID(req, testUserID)
		rec := dispatchGetJob(h, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status: got %d, want %d (body=%s)", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		if called {
			t.Error("Jobs.GetByID must not be called for a non-numeric id path value")
		}
	})
}
