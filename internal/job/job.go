// Package job provides an analysis-job repository backed by raw SQL against
// MySQL, per CLAUDE.md's no-ORM rule.
package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a lookup finds no matching job row (or one
// that exists but is not owned by the requesting user).
var ErrNotFound = errors.New("job: not found")

// ErrDuplicate is returned by Create when the user already has a non-failed
// job for the same youtube_url.
var ErrDuplicate = errors.New("job: duplicate submission")

// Job mirrors a row in the analysis_jobs table.
type Job struct {
	ID         uint64
	UserID     uint64
	YoutubeURL string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Metric mirrors a row in the participant_metrics table (minus its own id and
// the participant_id foreign key, which are implicit from context).
type Metric struct {
	Key   string
	Value float64
}

// Participant mirrors a row in the participants table, with its metrics
// attached.
type Participant struct {
	ID      uint64
	Label   string
	Metrics []Metric
}

// JobDetail is a Job plus its participants (with metrics) and summary (if
// any).
type JobDetail struct {
	Job
	Participants []Participant
	Summary      sql.NullString
}

// Create inserts a new analysis_jobs row for userID/youtubeURL with
// status='pending', unless the user already has a non-failed job for that
// exact URL, in which case it returns ErrDuplicate and no row is inserted.
func Create(ctx context.Context, db *sql.DB, userID uint64, youtubeURL string) (Job, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, fmt.Errorf("job create: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID uint64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM analysis_jobs WHERE user_id = ? AND youtube_url = ? AND status <> 'failed' FOR UPDATE`,
		userID, youtubeURL,
	).Scan(&existingID)
	switch {
	case err == nil:
		return Job{}, fmt.Errorf("job create: %w", ErrDuplicate)
	case errors.Is(err, sql.ErrNoRows):
		// No conflicting row; proceed with insert.
	default:
		return Job{}, fmt.Errorf("job create: duplicate check: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO analysis_jobs (user_id, youtube_url, status) VALUES (?, ?, 'pending')`,
		userID, youtubeURL,
	)
	if err != nil {
		return Job{}, fmt.Errorf("job create: insert: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Job{}, fmt.Errorf("job create: last insert id: %w", err)
	}

	var created Job
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id, youtube_url, status, created_at, updated_at FROM analysis_jobs WHERE id = ?`,
		id,
	).Scan(&created.ID, &created.UserID, &created.YoutubeURL, &created.Status, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("job create: reload: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Job{}, fmt.Errorf("job create: commit: %w", err)
	}

	return created, nil
}

// ListByUser returns all of userID's jobs, newest-first by created_at. It
// always returns a non-nil slice, even when the user has no jobs.
func ListByUser(ctx context.Context, db *sql.DB, userID uint64) ([]Job, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id, youtube_url, status, created_at, updated_at FROM analysis_jobs WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("job list by user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := []Job{}
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.UserID, &j.YoutubeURL, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("job list by user: scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("job list by user: rows: %w", err)
	}

	return jobs, nil
}

// GetByID returns the job identified by id, scoped to userID (wrapping
// sql.ErrNoRows into ErrNotFound so a job belonging to another user is
// indistinguishable from one that doesn't exist at all), along with its
// participants (each with their metrics) and summary, if any.
func GetByID(ctx context.Context, db *sql.DB, id, userID uint64) (JobDetail, error) {
	var detail JobDetail
	err := db.QueryRowContext(ctx,
		`SELECT id, user_id, youtube_url, status, created_at, updated_at FROM analysis_jobs WHERE id = ? AND user_id = ?`,
		id, userID,
	).Scan(&detail.ID, &detail.UserID, &detail.YoutubeURL, &detail.Status, &detail.CreatedAt, &detail.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobDetail{}, fmt.Errorf("job get by id: %w", ErrNotFound)
		}
		return JobDetail{}, fmt.Errorf("job get by id: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT p.id, p.label, m.metric_key, m.metric_value
		 FROM participants p
		 LEFT JOIN participant_metrics m ON m.participant_id = p.id
		 WHERE p.job_id = ?
		 ORDER BY p.id`,
		id,
	)
	if err != nil {
		return JobDetail{}, fmt.Errorf("job get by id: participants query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	participants := []Participant{}
	var current *Participant
	for rows.Next() {
		var (
			pID       uint64
			label     string
			metricKey sql.NullString
			metricVal sql.NullFloat64
		)
		if err := rows.Scan(&pID, &label, &metricKey, &metricVal); err != nil {
			return JobDetail{}, fmt.Errorf("job get by id: participants scan: %w", err)
		}

		if current == nil || current.ID != pID {
			participants = append(participants, Participant{ID: pID, Label: label, Metrics: []Metric{}})
			current = &participants[len(participants)-1]
		}

		if metricKey.Valid {
			current.Metrics = append(current.Metrics, Metric{Key: metricKey.String, Value: metricVal.Float64})
		}
	}
	if err := rows.Err(); err != nil {
		return JobDetail{}, fmt.Errorf("job get by id: participants rows: %w", err)
	}
	detail.Participants = participants

	var summary sql.NullString
	err = db.QueryRowContext(ctx, `SELECT summary FROM job_summaries WHERE job_id = ?`, id).Scan(&summary)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return JobDetail{}, fmt.Errorf("job get by id: summary: %w", err)
	}
	detail.Summary = summary

	return detail, nil
}
