package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubPinger is a local Pinger that always returns nil (healthy).
type stubPinger struct{}

func (stubPinger) Ping(_ context.Context) error { return nil }

func TestBuildServer_NoPanic(t *testing.T) {
	// buildServer's signature grew in specs/user-auth/plan.md step 6 to also
	// accept the auth dependencies (see TestBuildServer_RegistersAuthRoutes
	// below); this call site is updated to match so both tests compile
	// against the one buildServer signature.
	mux := buildServer(
		stubPinger{}, stubPinger{},
		stubAuthOTPRequester{}, stubAuthOTPVerifier{},
		stubAuthSessionCreator{}, stubAuthSessionDeleter{}, stubAuthSessionValidator{},
		stubAuthUserFinder{},
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
