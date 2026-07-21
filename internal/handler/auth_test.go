package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g4uk/kai/internal/otp"
	"github.com/g4uk/kai/internal/session"
)

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/user-auth/plan.md step 5)
//
// internal/handler/auth.go does not exist yet. This test file references the
// following production identifiers that auth.go must define; until it does,
// this package fails to compile (expected, correct red state):
//
//	type OTPRequestHandler struct{ OTP OTPRequester }
//	func (h *OTPRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
//
//	type OTPVerifyHandler struct{
//	    OTP      OTPVerifier
//	    Sessions SessionCreator
//	    Users    UserFinder
//	}
//	func (h *OTPVerifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
//
//	type LogoutHandler struct{ Sessions SessionDeleter }
//	func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
//
//	func SessionMiddleware(next http.Handler, sessions SessionValidator) http.Handler
//	func UserIDFromContext(ctx context.Context) (uint64, bool)
//
// The six consumer-defined interfaces below (OTPRequester, OTPVerifier,
// SessionCreator, SessionDeleter, UserFinder, SessionValidator) are declared
// HERE, in the test file, only because auth.go doesn't exist yet to own them
// (mirroring the Pinger pattern in health.go, where the interface lives in the
// production file, not the test file). When auth.go is implemented it MUST
// define these same interfaces (same names, same method signatures) — the
// implementer must then DELETE this block from auth_test.go to avoid
// duplicate declarations in the same package.
//
// ASSUMPTION (documented, to be reconciled by the implementer): plan.md step 5
// lists `OTPRequester.Request(ctx, phone) error`, but step 3's narrative says
// Request "returns the plaintext code to the caller for the stub log line —
// logging itself happens in the handler, not this package." Only the handler
// can log the code if Request returns it, so this test assumes:
// `OTPRequester.Request(ctx, phone string) (string, error)`.
//
// ASSUMPTION: the session cookie name is "session_id" (not pinned by the
// spec/plan; chosen for both OTPVerifyHandler's Set-Cookie and
// SessionMiddleware/LogoutHandler's cookie read).
//
// ASSUMPTION: POST /auth/logout without a session cookie is handled
// gracefully — no SessionDeleter call is required, and the response is 200
// (idempotent logout), since the spec only defines behavior for the
// with-cookie case (criterion 12).
// ----------------------------------------------------------------------------

const testSessionCookieName = "session_id"

// ---- stubs ------------------------------------------------------------

type stubOTPRequester struct {
	code string
	err  error
}

func (s stubOTPRequester) Request(_ context.Context, _ string) (string, error) {
	return s.code, s.err
}

type stubOTPVerifier struct{ err error }

func (s stubOTPVerifier) Verify(_ context.Context, _, _ string) error { return s.err }

type stubSessionCreator struct {
	id  string
	err error
}

func (s stubSessionCreator) Create(_ context.Context, _ uint64) (string, error) {
	return s.id, s.err
}

type stubSessionDeleter struct {
	err     error
	calledP *bool
}

func (s stubSessionDeleter) Delete(_ context.Context, _ string) error {
	if s.calledP != nil {
		*s.calledP = true
	}
	return s.err
}

type stubUserFinder struct {
	userID uint64
	err    error
}

func (s stubUserFinder) GetOrCreateByPhone(_ context.Context, _ string) (uint64, error) {
	return s.userID, s.err
}

type stubSessionValidator struct {
	userID uint64
	err    error
}

func (s stubSessionValidator) Validate(_ context.Context, _ string) (uint64, error) {
	return s.userID, s.err
}

// ---- OTPRequestHandler --------------------------------------------------

func TestOTPRequestHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		requester  OTPRequester
		wantStatus int
	}{
		{
			name:       "malformed phone number",
			body:       `{"phone_number":"not-a-phone"}`,
			requester:  stubOTPRequester{code: "123456"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid E.164 phone number accepted",
			body:       `{"phone_number":"+15551234567"}`,
			requester:  stubOTPRequester{code: "123456"},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "rate limited",
			body:       `{"phone_number":"+15551234567"}`,
			requester:  stubOTPRequester{err: otp.ErrRateLimited},
			wantStatus: http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &OTPRequestHandler{OTP: tt.requester}

			req := httptest.NewRequest(http.MethodPost, "/auth/otp/request", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusAccepted && strings.Contains(rec.Body.String(), tt.requester.(stubOTPRequester).code) {
				t.Errorf("response body must not contain the OTP value, got %q", rec.Body.String())
			}
		})
	}
}

// ---- OTPVerifyHandler ---------------------------------------------------

func TestOTPVerifyHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		verifier   OTPVerifier
		sessions   SessionCreator
		users      UserFinder
		wantStatus int
	}{
		{
			name:       "malformed phone number",
			body:       `{"phone_number":"not-a-phone","code":"123456"}`,
			verifier:   stubOTPVerifier{},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "correct code issues session and cookie",
			body:       `{"phone_number":"+15551234567","code":"123456"}`,
			verifier:   stubOTPVerifier{err: nil},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong code",
			body:       `{"phone_number":"+15551234567","code":"000000"}`,
			verifier:   stubOTPVerifier{err: otp.ErrMismatch},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "expired code",
			body:       `{"phone_number":"+15551234567","code":"123456"}`,
			verifier:   stubOTPVerifier{err: otp.ErrExpired},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "replay of consumed code",
			body:       `{"phone_number":"+15551234567","code":"123456"}`,
			verifier:   stubOTPVerifier{err: otp.ErrNotFound},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "too many attempts",
			body:       `{"phone_number":"+15551234567","code":"123456"}`,
			verifier:   stubOTPVerifier{err: otp.ErrTooManyAttempts},
			sessions:   stubSessionCreator{id: "sess-1"},
			users:      stubUserFinder{userID: 1},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &OTPVerifyHandler{OTP: tt.verifier, Sessions: tt.sessions, Users: tt.users}

			req := httptest.NewRequest(http.MethodPost, "/auth/otp/verify", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				cookies := rec.Result().Cookies()
				var found *http.Cookie
				for _, c := range cookies {
					if c.Name == testSessionCookieName {
						found = c
						break
					}
				}
				if found == nil {
					t.Fatalf("no %q cookie set on success response; cookies: %v", testSessionCookieName, cookies)
				}
				if !found.HttpOnly {
					t.Error("session cookie must be HttpOnly")
				}
				if !found.Secure {
					t.Error("session cookie must be Secure")
				}
				if found.SameSite != http.SameSiteStrictMode {
					t.Errorf("session cookie SameSite = %v, want SameSiteStrictMode", found.SameSite)
				}
				if found.Value == "" {
					t.Error("session cookie value must not be empty")
				}
			}
		})
	}
}

// ---- LogoutHandler --------------------------------------------------------

func TestLogoutHandler(t *testing.T) {
	t.Run("logout with valid session cookie clears the cookie", func(t *testing.T) {
		called := false
		h := &LogoutHandler{Sessions: stubSessionDeleter{calledP: &called}}

		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
		}
		if !called {
			t.Error("expected SessionDeleter.Delete to be called when a session cookie is present")
		}

		var cleared *http.Cookie
		for _, c := range rec.Result().Cookies() {
			if c.Name == testSessionCookieName {
				cleared = c
			}
		}
		if cleared == nil {
			t.Fatal("expected a cleared session cookie in the response")
		}
		// Per spec criterion 12 ("Max-Age=0"): Go's http.Cookie serializes
		// MaxAge<0 ("delete now") as the literal wire text "Max-Age=0", and
		// parsing that text back yields Cookie.MaxAge == -1. So the correct
		// assertion here is MaxAge <= 0, not MaxAge == 0.
		if cleared.MaxAge > 0 {
			t.Errorf("cleared cookie MaxAge = %d, want <= 0 (cookie deletion)", cleared.MaxAge)
		}
	})

	t.Run("logout without a session cookie still handles gracefully", func(t *testing.T) {
		h := &LogoutHandler{Sessions: stubSessionDeleter{}}

		req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status: got %d, want %d (graceful no-op logout)", rec.Code, http.StatusOK)
		}
	})
}

// ---- SessionMiddleware ------------------------------------------------

func TestSessionMiddleware(t *testing.T) {
	t.Run("valid session passes through with user_id in context", func(t *testing.T) {
		var gotUserID uint64
		var gotOK bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUserID, gotOK = UserIDFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		mw := SessionMiddleware(next, stubSessionValidator{userID: 99})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "sess-1"})
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
		}
		if !gotOK {
			t.Fatal("expected user_id to be present in downstream handler's context")
		}
		if gotUserID != 99 {
			t.Errorf("user_id in context = %d, want %d", gotUserID, 99)
		}
	})

	t.Run("missing session cookie rejected with 401", func(t *testing.T) {
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		mw := SessionMiddleware(next, stubSessionValidator{userID: 99})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if nextCalled {
			t.Error("next handler must not be called when the session cookie is missing")
		}
	})

	t.Run("invalid/unknown session rejected with 401", func(t *testing.T) {
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		mw := SessionMiddleware(next, stubSessionValidator{err: session.ErrNotFound})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: testSessionCookieName, Value: "bogus"})
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if nextCalled {
			t.Error("next handler must not be called when the session is invalid/unknown")
		}
	})
}
