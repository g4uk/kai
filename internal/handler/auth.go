package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/g4uk/kai/internal/otp"
)

// sessionCookieName is the name of the cookie used to carry the session ID.
const sessionCookieName = "session_id"

// e164Pattern matches E.164-formatted phone numbers (a leading '+', a
// non-zero first digit, and up to 15 digits total).
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// ---- consumer-defined interfaces (mirrors the Pinger pattern in health.go) --

// OTPRequester generates and stores a one-time code for phone, returning the
// plaintext code so the handler can log it (stub for SMS delivery).
type OTPRequester interface {
	Request(ctx context.Context, phone string) (code string, err error)
}

// OTPVerifier checks a submitted code against the stored code for phone.
type OTPVerifier interface {
	Verify(ctx context.Context, phone, code string) error
}

// SessionCreator issues a new server-side session for userID.
type SessionCreator interface {
	Create(ctx context.Context, userID uint64) (sessionID string, err error)
}

// SessionDeleter invalidates a server-side session.
type SessionDeleter interface {
	Delete(ctx context.Context, sessionID string) error
}

// UserFinder resolves a phone number to a user ID, auto-provisioning an
// account on first successful verify.
type UserFinder interface {
	GetOrCreateByPhone(ctx context.Context, phone string) (userID uint64, err error)
}

// SessionValidator resolves a session ID to its owning user ID.
type SessionValidator interface {
	Validate(ctx context.Context, sessionID string) (userID uint64, err error)
}

// ---- context plumbing -------------------------------------------------

type contextKey struct{ name string }

var userIDContextKey = &contextKey{name: "user_id"}

// UserIDFromContext returns the user ID attached by SessionMiddleware, if
// any.
func UserIDFromContext(ctx context.Context) (uint64, bool) {
	v, ok := ctx.Value(userIDContextKey).(uint64)
	return v, ok
}

// ---- request/response bodies -------------------------------------------

type otpRequestBody struct {
	PhoneNumber string `json:"phone_number"`
}

type otpVerifyBody struct {
	PhoneNumber string `json:"phone_number"`
	Code        string `json:"code"`
}

// ---- OTPRequestHandler ---------------------------------------------------

// OTPRequestHandler handles POST /auth/otp/request.
type OTPRequestHandler struct {
	OTP OTPRequester
}

func (h *OTPRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body otpRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !e164Pattern.MatchString(body.PhoneNumber) {
		http.Error(w, "invalid phone_number", http.StatusBadRequest)
		return
	}

	code, err := h.OTP.Request(r.Context(), body.PhoneNumber)
	if err != nil {
		if errors.Is(err, otp.ErrRateLimited) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "request failed", http.StatusInternalServerError)
		return
	}

	// TEMPORARY STAND-IN for real SMS delivery (see specs/user-auth/spec.md
	// non-scope): logs the plaintext OTP to stdout. Must not be treated as a
	// production log of user secrets.
	slog.Info("STUB SMS: OTP generated (temporary stand-in, not production SMS delivery)", "phone_number", body.PhoneNumber, "code", code)

	w.WriteHeader(http.StatusAccepted)
}

// ---- OTPVerifyHandler ---------------------------------------------------

// OTPVerifyHandler handles POST /auth/otp/verify.
type OTPVerifyHandler struct {
	OTP      OTPVerifier
	Sessions SessionCreator
	Users    UserFinder
}

func (h *OTPVerifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body otpVerifyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !e164Pattern.MatchString(body.PhoneNumber) {
		http.Error(w, "invalid phone_number", http.StatusBadRequest)
		return
	}

	if err := h.OTP.Verify(r.Context(), body.PhoneNumber, body.Code); err != nil {
		if errors.Is(err, otp.ErrMismatch) || errors.Is(err, otp.ErrExpired) ||
			errors.Is(err, otp.ErrNotFound) || errors.Is(err, otp.ErrTooManyAttempts) {
			http.Error(w, "invalid or expired code", http.StatusUnauthorized)
			return
		}
		http.Error(w, "verify failed", http.StatusInternalServerError)
		return
	}

	userID, err := h.Users.GetOrCreateByPhone(r.Context(), body.PhoneNumber)
	if err != nil {
		http.Error(w, "verify failed", http.StatusInternalServerError)
		return
	}

	sessionID, err := h.Sessions.Create(r.Context(), userID)
	if err != nil {
		http.Error(w, "verify failed", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	w.WriteHeader(http.StatusOK)
}

// ---- LogoutHandler --------------------------------------------------------

// LogoutHandler handles POST /auth/logout.
type LogoutHandler struct {
	Sessions SessionDeleter
}

func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		// No session cookie: idempotent no-op (spec only defines behavior for
		// the with-cookie case).
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.Sessions.Delete(r.Context(), cookie.Value); err != nil {
		http.Error(w, "logout failed", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	w.WriteHeader(http.StatusOK)
}

// ---- MeHandler ------------------------------------------------------------

// MeHandler handles GET /auth/me, wrapped in SessionMiddleware. Reaching
// ServeHTTP at all (past SessionMiddleware) is the only fact it reports, so
// it does nothing but write 204.
type MeHandler struct{}

func (h *MeHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// ---- SessionMiddleware ------------------------------------------------

// SessionMiddleware resolves the session cookie to a user_id via sessions
// and attaches it to the request context; missing/invalid/expired sessions
// are rejected with 401 without calling next.
func SessionMiddleware(next http.Handler, sessions SessionValidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		userID, err := sessions.Validate(r.Context(), cookie.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
