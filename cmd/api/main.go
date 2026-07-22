package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/handler"
	"github.com/g4uk/kai/internal/job"
	"github.com/g4uk/kai/internal/otp"
	"github.com/g4uk/kai/internal/redisconn"
	"github.com/g4uk/kai/internal/session"
	"github.com/g4uk/kai/internal/user"
)

// Concrete auth values per specs/user-auth/plan.md: session TTL = 30 days;
// OTP TTL = 5 min; OTP request limit = 5/hour/phone; OTP verify attempt
// limit = 5/code.
const (
	sessionTTL       = 30 * 24 * time.Hour
	otpCodeTTL       = 5 * time.Minute
	otpRequestWindow = time.Hour
	otpMaxRequests   = 5
	otpMaxAttempts   = 5
)

type dbPinger struct{ db *sql.DB }

func (p *dbPinger) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

type redisPinger struct{ c *redis.Client }

func (p *redisPinger) Ping(ctx context.Context) error { return p.c.Ping(ctx).Err() }

// userFinder adapts internal/user's phone-keyed repository to
// handler.UserFinder, auto-provisioning an account on a phone number's first
// successful OTP verify, per specs/user-auth/spec.md.
type userFinder struct{ db *sql.DB }

func (f *userFinder) GetOrCreateByPhone(ctx context.Context, phone string) (uint64, error) {
	u, err := user.GetByPhone(ctx, f.db, phone)
	if errors.Is(err, user.ErrNotFound) {
		u, err = user.Create(ctx, f.db, phone)
	}
	if err != nil {
		return 0, err
	}
	return u.ID, nil
}

// jobStore adapts internal/job's repository functions to
// handler.JobCreator/JobLister/JobGetter, converting internal/job types into
// internal/handler's local types per specs/jobs-api/plan.md step 3 (keeping
// internal/handler free of concrete repo types, mirroring userFinder above).
type jobStore struct{ db *sql.DB }

func (s *jobStore) Create(ctx context.Context, userID uint64, youtubeURL string) (handler.Job, error) {
	j, err := job.Create(ctx, s.db, userID, youtubeURL)
	if err != nil {
		return handler.Job{}, err
	}
	return toHandlerJob(j), nil
}

func (s *jobStore) ListByUser(ctx context.Context, userID uint64) ([]handler.Job, error) {
	jobs, err := job.ListByUser(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	out := make([]handler.Job, len(jobs))
	for i, j := range jobs {
		out[i] = toHandlerJob(j)
	}
	return out, nil
}

func (s *jobStore) GetByID(ctx context.Context, id, userID uint64) (handler.JobDetail, error) {
	detail, err := job.GetByID(ctx, s.db, id, userID)
	if err != nil {
		return handler.JobDetail{}, err
	}

	participants := make([]handler.Participant, len(detail.Participants))
	for i, p := range detail.Participants {
		metrics := make([]handler.Metric, len(p.Metrics))
		for j, m := range p.Metrics {
			metrics[j] = handler.Metric{Key: m.Key, Value: m.Value}
		}
		participants[i] = handler.Participant{ID: p.ID, Label: p.Label, Metrics: metrics}
	}

	var summary *string
	if detail.Summary.Valid {
		s := detail.Summary.String
		summary = &s
	}

	return handler.JobDetail{
		Job:          toHandlerJob(detail.Job),
		Participants: participants,
		Summary:      summary,
	}, nil
}

func toHandlerJob(j job.Job) handler.Job {
	return handler.Job{
		ID:         j.ID,
		YoutubeURL: j.YoutubeURL,
		Status:     j.Status,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
	}
}

// jobStoreDeps is implemented by any value satisfying all three job-handler
// interfaces at once (e.g. *jobStore in main(), or a combined stub in
// tests) — mirroring how otpService already satisfies both OTPRequester and
// OTPVerifier.
type jobStoreDeps interface {
	handler.JobCreator
	handler.JobLister
	handler.JobGetter
}

func buildServer(
	database handler.Pinger,
	redisConn handler.Pinger,
	otpRequester handler.OTPRequester,
	otpVerifier handler.OTPVerifier,
	sessionCreator handler.SessionCreator,
	sessionDeleter handler.SessionDeleter,
	sessionValidator handler.SessionValidator,
	users handler.UserFinder,
	jobs jobStoreDeps,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", &handler.HealthHandler{DB: database, Redis: redisConn})
	mux.Handle("POST /auth/otp/request", &handler.OTPRequestHandler{OTP: otpRequester})
	mux.Handle("POST /auth/otp/verify", &handler.OTPVerifyHandler{OTP: otpVerifier, Sessions: sessionCreator, Users: users})
	// NOTE: /auth/logout is intentionally NOT wrapped in SessionMiddleware
	// (deviates from plan.md step 6's literal wording) — LogoutHandler must
	// respond 200 as an idempotent no-op when no session cookie is present
	// (spec criterion 12's with-cookie case plus the documented assumption in
	// internal/handler/auth_test.go), and SessionMiddleware would instead
	// reject that request with 401 before LogoutHandler ever runs.
	// sessionValidator is accepted here for buildServer's signature (shared by
	// SessionMiddleware once a protected, non-auth route exists) but has no
	// consumer yet in this spec's scope.
	mux.Handle("POST /auth/logout", &handler.LogoutHandler{Sessions: sessionDeleter})
	mux.Handle("POST /jobs", handler.SessionMiddleware(&handler.CreateJobHandler{Jobs: jobs}, sessionValidator))
	mux.Handle("GET /jobs", handler.SessionMiddleware(&handler.ListJobsHandler{Jobs: jobs}, sessionValidator))
	mux.Handle("GET /jobs/{id}", handler.SessionMiddleware(&handler.GetJobHandler{Jobs: jobs}, sessionValidator))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	return mux
}

func main() {
	dsn := os.Getenv("DB_DSN")
	redisAddr := os.Getenv("REDIS_ADDR")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), db.MaxRetryBudget)
	defer dbCancel()
	sqlDB, err := db.Connect(dbCtx, dsn)
	if err != nil {
		slog.Error("db connect failed", "err", err)
		os.Exit(1)
	}

	if err := db.Up(sqlDB); err != nil {
		slog.Error("db migrate failed", "err", err)
		os.Exit(1)
	}

	redisCtx, redisCancel := context.WithTimeout(context.Background(), redisconn.MaxRetryBudget)
	defer redisCancel()
	redisClient, err := redisconn.Connect(redisCtx, redisAddr)
	if err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}

	otpService := &otp.Service{
		Client:        redisClient,
		CodeTTL:       otpCodeTTL,
		RequestWindow: otpRequestWindow,
		MaxRequests:   otpMaxRequests,
		MaxAttempts:   otpMaxAttempts,
	}
	sessionStore := &session.Store{Client: redisClient, TTL: sessionTTL}
	users := &userFinder{db: sqlDB}
	jobs := &jobStore{db: sqlDB}

	mux := buildServer(
		&dbPinger{db: sqlDB}, &redisPinger{c: redisClient},
		otpService, otpService,
		sessionStore, sessionStore, sessionStore,
		users,
		jobs,
	)

	slog.Info("starting api", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
