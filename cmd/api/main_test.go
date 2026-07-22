package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g4uk/kai/internal/handler"
)

// stubPinger is a local Pinger that always returns nil (healthy).
type stubPinger struct{}

func (stubPinger) Ping(_ context.Context) error { return nil }

func TestBuildServer_NoPanic(t *testing.T) {
	// buildServer's signature grew in specs/user-auth/plan.md step 6 to also
	// accept the auth dependencies (see TestBuildServer_RegistersAuthRoutes
	// below), and again in specs/jobs-api/plan.md step 3 to accept the jobs
	// dependency (see TestBuildServer_RegistersJobRoutes below); this call
	// site is updated to match so all three tests compile against the one
	// buildServer signature.
	mux := buildServer(
		stubPinger{}, stubPinger{},
		stubAuthOTPRequester{}, stubAuthOTPVerifier{},
		stubAuthSessionCreator{}, stubAuthSessionDeleter{}, stubAuthSessionValidator{},
		stubAuthUserFinder{},
		stubJobStore{},
	)
	if mux == nil {
		t.Fatal("buildServer returned nil mux")
	}
}

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/user-auth/plan.md step 6)
//
// buildServer's signature must grow to accept the auth dependencies so it can
// register POST /auth/otp/request, POST /auth/otp/verify and POST
// /auth/logout. plan.md explicitly defers the "positional params vs. options
// struct" call to implementation time (YAGNI), so this test documents the
// assumption it needs to compile against: buildServer gains six additional
// positional parameters, one per handler-layer interface from
// internal/handler/auth.go (OTPRequester, OTPVerifier, SessionCreator,
// SessionDeleter, SessionValidator, UserFinder), in that order, after the two
// existing Pinger params. If the implementer chooses an options struct
// instead, only this call site needs to change — the route-registration
// assertions below remain valid either way.
//
// The stub types below satisfy those interfaces structurally; they don't
// import internal/handler, so this test doesn't depend on auth.go's
// interfaces existing yet — only on buildServer's new signature.
// ----------------------------------------------------------------------------

type stubAuthOTPRequester struct{}

func (stubAuthOTPRequester) Request(_ context.Context, _ string) (string, error) {
	return "123456", nil
}

type stubAuthOTPVerifier struct{}

func (stubAuthOTPVerifier) Verify(_ context.Context, _, _ string) error { return nil }

type stubAuthSessionCreator struct{}

func (stubAuthSessionCreator) Create(_ context.Context, _ uint64) (string, error) {
	return "sess-1", nil
}

type stubAuthSessionDeleter struct{}

func (stubAuthSessionDeleter) Delete(_ context.Context, _ string) error { return nil }

type stubAuthSessionValidator struct{}

func (stubAuthSessionValidator) Validate(_ context.Context, _ string) (uint64, error) { return 1, nil }

type stubAuthUserFinder struct{}

func (stubAuthUserFinder) GetOrCreateByPhone(_ context.Context, _ string) (uint64, error) {
	return 1, nil
}

func TestBuildServer_RegistersAuthRoutes(t *testing.T) {
	mux := buildServer(
		stubPinger{}, stubPinger{},
		stubAuthOTPRequester{}, stubAuthOTPVerifier{},
		stubAuthSessionCreator{}, stubAuthSessionDeleter{}, stubAuthSessionValidator{},
		stubAuthUserFinder{},
		stubJobStore{},
	)
	if mux == nil {
		t.Fatal("buildServer returned nil mux")
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/auth/otp/request"},
		{http.MethodPost, "/auth/otp/verify"},
		{http.MethodPost, "/auth/logout"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Errorf("%s %s: got 404, want the route to be registered", rt.method, rt.path)
		}
	}
}

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/jobs-api/plan.md step 3)
//
// buildServer's signature must grow again to accept a single jobs dependency
// so it can register POST /jobs, GET /jobs and GET /jobs/{id}, each wrapped
// in handler.SessionMiddleware (mirroring how the auth routes above prove
// SessionMiddleware wraps them by asserting a 401 without a session cookie).
// This test documents the assumption it needs to compile against: buildServer
// gains ONE additional positional parameter, after the existing
// stubAuthUserFinder-shaped one, typed to accept a single value satisfying
// handler.JobCreator + handler.JobLister + handler.JobGetter together (per
// plan.md step 3's "jobStore-shaped parameter, reused for all three
// interfaces" — mirroring otpService already satisfying both OTPRequester and
// OTPVerifier). If the implementer instead adds three separate parameters,
// only this call site and stubJobStore below need to change.
//
// UNLIKE the existing stubAuth* stubs above (which don't import
// internal/handler because OTPRequester/SessionCreator/etc. only use
// primitive return types), stubJobCreator/stubJobLister/stubJobGetter below
// DO need to import internal/handler for the handler.Job/handler.JobDetail
// return types: Go's defined-type identity rules mean a locally-declared
// "look-alike" struct in package main would be a different type from
// handler.Job and would NOT structurally satisfy handler.JobCreator (unlike
// e.g. a bare uint64/string return, which is a built-in type shared by
// definition). main.go already imports internal/handler, so this isn't a new
// dependency for the package as a whole — only a new import line in this
// test file. Flagging so this isn't mistaken for scope creep.
// ----------------------------------------------------------------------------

type stubJobCreator struct{}

func (stubJobCreator) Create(_ context.Context, _ uint64, _ string) (handler.Job, error) {
	return handler.Job{ID: 1, YoutubeURL: "https://youtu.be/aaaaaaaaaaa", Status: "pending"}, nil
}

type stubJobLister struct{}

func (stubJobLister) ListByUser(_ context.Context, _ uint64) ([]handler.Job, error) {
	return []handler.Job{}, nil
}

type stubJobGetter struct{}

func (stubJobGetter) GetByID(_ context.Context, _, _ uint64) (handler.JobDetail, error) {
	return handler.JobDetail{Participants: []handler.Participant{}}, nil
}

// stubJobStore combines the three single-method stubs above (via embedding
// and method promotion) into one value satisfying handler.JobCreator,
// handler.JobLister and handler.JobGetter simultaneously, matching
// buildServer's assumed single "jobStore-shaped parameter" per the note
// above.
type stubJobStore struct {
	stubJobCreator
	stubJobLister
	stubJobGetter
}

const testJobsSessionCookieName = "session_id"

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/auth-me/plan.md step 2)
//
// buildServer must register GET /auth/me, wrapped in handler.SessionMiddleware
// (using the existing sessionValidator param, which previously had no
// consumer). This proves both that the route exists and that it's protected
// by SessionMiddleware (401 without a session cookie, 204 with one).
// ----------------------------------------------------------------------------

func TestBuildServer_RegistersAuthMeRoute(t *testing.T) {
	mux := buildServer(
		stubPinger{}, stubPinger{},
		stubAuthOTPRequester{}, stubAuthOTPVerifier{},
		stubAuthSessionCreator{}, stubAuthSessionDeleter{}, stubAuthSessionValidator{},
		stubAuthUserFinder{},
		stubJobStore{},
	)
	if mux == nil {
		t.Fatal("buildServer returned nil mux")
	}

	t.Run("GET /auth/me without session cookie is rejected with 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("GET /auth/me with valid session cookie returns 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: testJobsSessionCookieName, Value: "sess-1"})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusNoContent)
		}
	})
}

func TestBuildServer_RegistersJobRoutes(t *testing.T) {
	mux := buildServer(
		stubPinger{}, stubPinger{},
		stubAuthOTPRequester{}, stubAuthOTPVerifier{},
		stubAuthSessionCreator{}, stubAuthSessionDeleter{}, stubAuthSessionValidator{},
		stubAuthUserFinder{},
		stubJobStore{},
	)
	if mux == nil {
		t.Fatal("buildServer returned nil mux")
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/jobs"},
		{http.MethodGet, "/jobs"},
		{http.MethodGet, "/jobs/1"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path+" with valid session is registered", func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			req.AddCookie(&http.Cookie{Name: testJobsSessionCookieName, Value: "sess-1"})
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Errorf("%s %s: got 404, want the route to be registered", rt.method, rt.path)
			}
		})

		t.Run(rt.method+" "+rt.path+" without session cookie is rejected with 401", func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without session cookie: got %d, want %d (proving SessionMiddleware wraps this route)", rt.method, rt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}
