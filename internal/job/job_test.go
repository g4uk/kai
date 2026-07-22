package job

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/jobs-api/plan.md step 1)
//
// internal/job/job.go does not exist yet. This test file references the
// following production identifiers that job.go must define; until it does,
// this package fails to compile (expected, correct red state):
//
//	type Job struct {
//	    ID         uint64
//	    UserID     uint64
//	    YoutubeURL string
//	    Status     string
//	    CreatedAt  time.Time
//	    UpdatedAt  time.Time
//	}
//	type Participant struct { ID uint64; Label string; Metrics []Metric }
//	type Metric struct { Key string; Value float64 }
//	type JobDetail struct { Job; Participants []Participant; Summary sql.NullString }
//
//	var ErrNotFound error
//	var ErrDuplicate error
//
//	func Create(ctx context.Context, db *sql.DB, userID uint64, youtubeURL string) (Job, error)
//	func ListByUser(ctx context.Context, db *sql.DB, userID uint64) ([]Job, error)
//	func GetByID(ctx context.Context, db *sql.DB, id, userID uint64) (JobDetail, error)
//
// Gated on TEST_DSN (same convention as internal/user/user_test.go). Calls
// db.Up to ensure the schema (analysis_jobs/participants/participant_metrics/
// job_summaries from 001_initial_schema.sql) exists before exercising the
// repository, per plan.md step 1.
// ----------------------------------------------------------------------------

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/user"
)

// testDB mirrors internal/user/user_test.go's helper exactly; it is
// package-local (not shared) per the plan.md conventions note.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set; skipping integration test")
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Up(sqlDB); err != nil {
		t.Fatalf("db.Up: %v", err)
	}

	return sqlDB
}

// mustCreateUser creates a user row (analysis_jobs.user_id has a foreign key
// to users.id) and registers cleanup for it. Cleanup is LIFO, so any
// job/participant/summary cleanup registered after this call will run before
// the user row is deleted.
func mustCreateUser(t *testing.T, sqlDB *sql.DB, phone string) uint64 {
	t.Helper()

	u, err := user.Create(context.Background(), sqlDB, phone)
	if err != nil {
		t.Fatalf("user.Create(%q): %v", phone, err)
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
	})
	return u.ID
}

// cleanupJob deletes a job row and anything hanging off it (summary,
// participants, metrics), registered to run before the owning user is
// deleted (LIFO cleanup order).
func cleanupJob(t *testing.T, sqlDB *sql.DB, jobID uint64) {
	t.Helper()

	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM job_summaries WHERE job_id = ?`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM participant_metrics WHERE participant_id IN (SELECT id FROM participants WHERE job_id = ?)`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM participants WHERE job_id = ?`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM analysis_jobs WHERE id = ?`, jobID)
	})
}

// ---- Create -------------------------------------------------------------

func TestCreate_Succeeds(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993001")
	url := "https://www.youtube.com/watch?v=create-succeeds-1"

	created, err := Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, created.ID)

	if created.ID == 0 {
		t.Error("created.ID = 0, want non-zero auto-increment ID")
	}
	if created.Status != "pending" {
		t.Errorf("created.Status = %q, want %q", created.Status, "pending")
	}
	if created.UserID != userID {
		t.Errorf("created.UserID = %d, want %d", created.UserID, userID)
	}
	if created.YoutubeURL != url {
		t.Errorf("created.YoutubeURL = %q, want %q", created.YoutubeURL, url)
	}
}

func TestCreate_DuplicatePending(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993002")
	url := "https://www.youtube.com/watch?v=create-duplicate-1"

	first, err := Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	cleanupJob(t, sqlDB, first.ID)

	_, err = Create(ctx, sqlDB, userID, url)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Create for same pending user+URL: got err %v, want ErrDuplicate", err)
	}

	var count int
	row := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM analysis_jobs WHERE user_id = ? AND youtube_url = ?`,
		userID, url,
	)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("row count for user+URL = %d, want 1 (no second row inserted)", count)
	}
}

func TestCreate_AfterFailedAllowsResubmission(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993003")
	url := "https://www.youtube.com/watch?v=create-after-failed-1"

	first, err := Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	cleanupJob(t, sqlDB, first.ID)

	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE analysis_jobs SET status = 'failed' WHERE id = ?`, first.ID,
	); err != nil {
		t.Fatalf("manually marking job failed: %v", err)
	}

	second, err := Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("Create after prior job failed: got err %v, want success", err)
	}
	cleanupJob(t, sqlDB, second.ID)

	if second.ID == first.ID {
		t.Error("second.ID == first.ID, want a newly inserted row")
	}
	if second.Status != "pending" {
		t.Errorf("second.Status = %q, want %q", second.Status, "pending")
	}
}

// ---- ListByUser -----------------------------------------------------------

func TestListByUser_OrderedNewestFirst(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	ownerID := mustCreateUser(t, sqlDB, "+15559993004")
	otherID := mustCreateUser(t, sqlDB, "+15559993005")

	jobA, err := Create(ctx, sqlDB, ownerID, "https://www.youtube.com/watch?v=list-order-a")
	if err != nil {
		t.Fatalf("Create jobA: %v", err)
	}
	cleanupJob(t, sqlDB, jobA.ID)

	jobB, err := Create(ctx, sqlDB, ownerID, "https://www.youtube.com/watch?v=list-order-b")
	if err != nil {
		t.Fatalf("Create jobB: %v", err)
	}
	cleanupJob(t, sqlDB, jobB.ID)

	jobC, err := Create(ctx, sqlDB, ownerID, "https://www.youtube.com/watch?v=list-order-c")
	if err != nil {
		t.Fatalf("Create jobC: %v", err)
	}
	cleanupJob(t, sqlDB, jobC.ID)

	otherJob, err := Create(ctx, sqlDB, otherID, "https://www.youtube.com/watch?v=list-order-other")
	if err != nil {
		t.Fatalf("Create otherJob: %v", err)
	}
	cleanupJob(t, sqlDB, otherJob.ID)

	// Force deterministic created_at ordering (auto-timestamps from rapid
	// inserts may collide at second resolution).
	now := time.Now().UTC()
	for _, j := range []struct {
		id uint64
		at time.Time
	}{
		{jobA.ID, now.Add(-2 * time.Hour)},
		{jobB.ID, now.Add(-1 * time.Hour)},
		{jobC.ID, now},
	} {
		if _, err := sqlDB.ExecContext(ctx,
			`UPDATE analysis_jobs SET created_at = ? WHERE id = ?`, j.at, j.id,
		); err != nil {
			t.Fatalf("forcing created_at for job %d: %v", j.id, err)
		}
	}

	got, err := ListByUser(ctx, sqlDB, ownerID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	wantOrder := []uint64{jobC.ID, jobB.ID, jobA.ID}
	for i, wantID := range wantOrder {
		if got[i].ID != wantID {
			t.Errorf("got[%d].ID = %d, want %d (newest-first order)", i, got[i].ID, wantID)
		}
	}
	for _, j := range got {
		if j.ID == otherJob.ID {
			t.Error("ListByUser(ownerID) contains a job belonging to a different user")
		}
	}
}

func TestListByUser_EmptyUser(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993006")

	got, err := ListByUser(ctx, sqlDB, userID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if got == nil {
		t.Error("ListByUser with zero jobs returned nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// ---- GetByID ----------------------------------------------------------

func TestGetByID_WithParticipantsAndMetrics(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993007")

	j, err := Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=getbyid-participants-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, j.ID)

	aliceRes, err := sqlDB.ExecContext(ctx,
		`INSERT INTO participants (job_id, label) VALUES (?, ?)`, j.ID, "Alice")
	if err != nil {
		t.Fatalf("insert participant Alice: %v", err)
	}
	aliceID, err := aliceRes.LastInsertId()
	if err != nil {
		t.Fatalf("Alice LastInsertId: %v", err)
	}

	bobRes, err := sqlDB.ExecContext(ctx,
		`INSERT INTO participants (job_id, label) VALUES (?, ?)`, j.ID, "Bob")
	if err != nil {
		t.Fatalf("insert participant Bob: %v", err)
	}
	bobID, err := bobRes.LastInsertId()
	if err != nil {
		t.Fatalf("Bob LastInsertId: %v", err)
	}

	metricInserts := []struct {
		participantID int64
		key           string
		value         float64
	}{
		{aliceID, "strikes", 12.5},
		{aliceID, "speed", 3.2},
		{bobID, "strikes", 9},
	}
	for _, m := range metricInserts {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO participant_metrics (participant_id, metric_key, metric_value) VALUES (?, ?, ?)`,
			m.participantID, m.key, m.value,
		); err != nil {
			t.Fatalf("insert metric %+v: %v", m, err)
		}
	}

	detail, err := GetByID(ctx, sqlDB, j.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if len(detail.Participants) != 2 {
		t.Fatalf("len(detail.Participants) = %d, want 2", len(detail.Participants))
	}

	// Participants are ordered by p.id (insertion order) per plan.md step 1;
	// metric order within a participant is not specified, so compare as sets.
	if uint64(aliceID) != detail.Participants[0].ID {
		t.Errorf("Participants[0].ID = %d, want %d (Alice, inserted first)", detail.Participants[0].ID, aliceID)
	}
	if detail.Participants[0].Label != "Alice" {
		t.Errorf("Participants[0].Label = %q, want %q", detail.Participants[0].Label, "Alice")
	}
	wantAliceMetrics := map[string]float64{"strikes": 12.5, "speed": 3.2}
	assertMetricSet(t, "Alice", detail.Participants[0].Metrics, wantAliceMetrics)

	if uint64(bobID) != detail.Participants[1].ID {
		t.Errorf("Participants[1].ID = %d, want %d (Bob, inserted second)", detail.Participants[1].ID, bobID)
	}
	if detail.Participants[1].Label != "Bob" {
		t.Errorf("Participants[1].Label = %q, want %q", detail.Participants[1].Label, "Bob")
	}
	wantBobMetrics := map[string]float64{"strikes": 9}
	assertMetricSet(t, "Bob", detail.Participants[1].Metrics, wantBobMetrics)
}

func assertMetricSet(t *testing.T, who string, got []Metric, want map[string]float64) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("%s: len(Metrics) = %d, want %d", who, len(got), len(want))
	}
	gotMap := make(map[string]float64, len(got))
	for _, m := range got {
		gotMap[m.Key] = m.Value
	}
	for k, v := range want {
		gv, ok := gotMap[k]
		if !ok {
			t.Errorf("%s: missing metric %q", who, k)
			continue
		}
		if gv != v {
			t.Errorf("%s: metric %q = %v, want %v", who, k, gv, v)
		}
	}
}

func TestGetByID_ZeroParticipants(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993008")

	j, err := Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=getbyid-zero-participants-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, j.ID)

	detail, err := GetByID(ctx, sqlDB, j.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if detail.Participants == nil {
		t.Error("Participants = nil, want non-nil empty slice")
	}
	if len(detail.Participants) != 0 {
		t.Errorf("len(Participants) = %d, want 0", len(detail.Participants))
	}
}

func TestGetByID_NoSummary(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993009")

	j, err := Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=getbyid-no-summary-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, j.ID)

	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO participants (job_id, label) VALUES (?, ?)`, j.ID, "Solo",
	); err != nil {
		t.Fatalf("insert participant: %v", err)
	}

	detail, err := GetByID(ctx, sqlDB, j.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if detail.Summary.Valid {
		t.Errorf("Summary.Valid = true, want false (no job_summaries row)")
	}
}

func TestGetByID_WithSummary(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993010")

	j, err := Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=getbyid-with-summary-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, j.ID)

	wantSummary := "close bout, 4-3 on point differential"
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO job_summaries (job_id, summary) VALUES (?, ?)`, j.ID, wantSummary,
	); err != nil {
		t.Fatalf("insert job_summaries: %v", err)
	}

	detail, err := GetByID(ctx, sqlDB, j.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !detail.Summary.Valid {
		t.Fatal("Summary.Valid = false, want true (job_summaries row exists)")
	}
	if detail.Summary.String != wantSummary {
		t.Errorf("Summary.String = %q, want %q", detail.Summary.String, wantSummary)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	userID := mustCreateUser(t, sqlDB, "+15559993011")

	_, err := GetByID(ctx, sqlDB, 999999999, userID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID nonexistent id: got err %v, want ErrNotFound", err)
	}
}

func TestGetByID_ForeignTenant(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()
	ownerID := mustCreateUser(t, sqlDB, "+15559993012")
	otherID := mustCreateUser(t, sqlDB, "+15559993013")

	j, err := Create(ctx, sqlDB, ownerID, "https://www.youtube.com/watch?v=getbyid-foreign-tenant-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cleanupJob(t, sqlDB, j.ID)

	_, err = GetByID(ctx, sqlDB, j.ID, otherID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetByID for a job owned by a different user: got err %v, want ErrNotFound (indistinguishable from nonexistent)", err)
	}
}
