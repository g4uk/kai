package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubPinger is a Pinger that returns a fixed error (nil = healthy).
type stubPinger struct{ err error }

func (s stubPinger) Ping(_ context.Context) error { return s.err }

func TestHealthHandler(t *testing.T) {
	errDown := errors.New("connection refused")

	tests := []struct {
		name              string
		mysqlErr          error
		redisErr          error
		wantStatus        int
		wantMySQL         string
		wantRedis         string
		wantState         string
		wantErrorNonEmpty bool
	}{
		{
			name:              "both ok",
			mysqlErr:          nil,
			redisErr:          nil,
			wantStatus:        http.StatusOK,
			wantMySQL:         "ok",
			wantRedis:         "ok",
			wantState:         "ok",
			wantErrorNonEmpty: false,
		},
		{
			name:              "mysql down",
			mysqlErr:          errDown,
			redisErr:          nil,
			wantStatus:        http.StatusServiceUnavailable,
			wantMySQL:         "error",
			wantRedis:         "ok",
			wantState:         "error",
			wantErrorNonEmpty: true,
		},
		{
			name:              "redis down",
			mysqlErr:          nil,
			redisErr:          errDown,
			wantStatus:        http.StatusServiceUnavailable,
			wantMySQL:         "ok",
			wantRedis:         "error",
			wantState:         "error",
			wantErrorNonEmpty: true,
		},
		{
			name:              "both down",
			mysqlErr:          errDown,
			redisErr:          errDown,
			wantStatus:        http.StatusServiceUnavailable,
			wantMySQL:         "error",
			wantRedis:         "error",
			wantState:         "error",
			wantErrorNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HealthHandler{
				DB:    stubPinger{err: tt.mysqlErr},
				Redis: stubPinger{err: tt.redisErr},
			}

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}

			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if got := body["mysql"]; got != tt.wantMySQL {
				t.Errorf("mysql field: got %q, want %q", got, tt.wantMySQL)
			}
			if got := body["redis"]; got != tt.wantRedis {
				t.Errorf("redis field: got %q, want %q", got, tt.wantRedis)
			}
			if got := body["status"]; got != tt.wantState {
				t.Errorf("status field: got %q, want %q", got, tt.wantState)
			}
			if tt.wantErrorNonEmpty && body["error"] == "" {
				t.Errorf("error field: want non-empty string, got empty")
			}
			if !tt.wantErrorNonEmpty && body["error"] != "" {
				t.Errorf("error field: want empty (omitempty), got %q", body["error"])
			}
		})
	}
}
