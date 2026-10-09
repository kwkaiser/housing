package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobDegraded  JobStatus = "degraded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

func (s JobStatus) Active() bool {
	return s == JobQueued || s == JobRunning
}

var ErrJobNotFound = errors.New("job not found")

type Job struct {
	ID           int64
	Kind         string
	Trigger      string
	CollectionID string
	ProfileID    string
	Params       json.RawMessage
	Status       JobStatus
	CreatedAt    time.Time
	StartedAt    time.Time
	FinishedAt   time.Time
	Error        string
	CostUSD      float64
	Result       json.RawMessage
}

type JobEvent struct {
	JobID   int64
	Seq     int
	At      time.Time
	Level   slog.Level
	Message string
	Record  json.RawMessage
}

type JobFilter struct {
	Kind         string
	CollectionID string
	ProfileID    string
	Status       JobStatus
	Limit        int
}

type EnqueueRule struct {
	UniqueActive bool
	NoneSince    time.Time
}

const jobTimeLayout = "2006-01-02T15:04:05.000000000Z"

func jobTime(t time.Time) string {
	return t.UTC().Format(jobTimeLayout)
}

const jobColumns = `id, kind, trigger, coalesce(collection_id, ''), coalesce(profile_id, ''), params, status,
	created_at, coalesce(started_at, ''), coalesce(finished_at, ''), error, cost_usd, coalesce(result, '')`

func (s *Store) EnqueueJob(ctx context.Context, j Job, rule EnqueueRule) (int64, bool, error) {
	params := string(j.Params)
	if params == "" {
		params = "{}"
	}
	var conds []string
	args := []any{j.Kind, j.Trigger, nullString(j.CollectionID), nullString(j.ProfileID), params, JobQueued, jobTime(j.CreatedAt)}
	if rule.UniqueActive {
		conds = append(conds, `NOT EXISTS (SELECT 1 FROM jobs WHERE kind = ?1 AND collection_id IS ?3 AND status IN ('queued', 'running'))`)
	}
	if !rule.NoneSince.IsZero() {
		conds = append(conds, `NOT EXISTS (SELECT 1 FROM jobs WHERE kind = ?1 AND collection_id IS ?3 AND created_at >= ?8)`)
		args = append(args, jobTime(rule.NoneSince))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs (kind, trigger, collection_id, profile_id, params, status, created_at)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6, ?7 `+where, args...)
	if err != nil {
		return 0, false, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, false, err
	} else if n == 1 {
		id, err := res.LastInsertId()
		return id, true, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM jobs WHERE kind = ? AND collection_id IS ? ORDER BY status IN ('queued', 'running') DESC, id DESC LIMIT 1`,
		j.Kind, nullString(j.CollectionID)).Scan(&id)
	return id, false, err
}

func (s *Store) ClaimJob(ctx context.Context, at time.Time) (Job, bool, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `
		UPDATE jobs SET status = ?, started_at = ?
		WHERE id = (SELECT id FROM jobs WHERE status = ? ORDER BY id LIMIT 1)
		RETURNING `+jobColumns, JobRunning, jobTime(at), JobQueued))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	return j, err == nil, err
}

func (s *Store) AppendJobEvent(ctx context.Context, e JobEvent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO job_events (job_id, seq, at, level, message, record)
		SELECT ?1, coalesce(max(seq), 0) + 1, ?2, ?3, ?4, ?5 FROM job_events WHERE job_id = ?1`,
		e.JobID, jobTime(e.At), e.Level.String(), e.Message, string(e.Record))
	return err
}

func (s *Store) FinishJob(ctx context.Context, j Job) error {
	if j.Status.Active() {
		return fmt.Errorf("job %d: cannot finish with status %s", j.ID, j.Status)
	}
	var result any
	if len(j.Result) > 0 {
		result = string(j.Result)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, finished_at = ?, error = ?, cost_usd = ?, result = ? WHERE id = ?`,
		j.Status, jobTime(j.FinishedAt), j.Error, j.CostUSD, result, j.ID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: %d", ErrJobNotFound, j.ID)
	}
	return nil
}

func (s *Store) FailRunningJobs(ctx context.Context, reason string, at time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET status = ?, finished_at = ?, error = ? WHERE status = ?`,
		JobFailed, jobTime(at), reason, JobRunning)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (s *Store) Job(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, fmt.Errorf("%w: %d", ErrJobNotFound, id)
	}
	return j, err
}

func (s *Store) Jobs(ctx context.Context, f JobFilter) ([]Job, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobColumns+` FROM jobs
		WHERE (?1 = '' OR kind = ?1) AND (?2 = '' OR collection_id = ?2) AND (?3 = '' OR profile_id = ?3) AND (?5 = '' OR status = ?5)
		ORDER BY id DESC LIMIT ?4`,
		f.Kind, f.CollectionID, f.ProfileID, limit, f.Status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) ActiveJob(ctx context.Context, kind, collectionID string) (Job, bool, error) {
	return s.oneJob(ctx, `WHERE kind = ? AND collection_id = ? AND status IN ('queued', 'running') ORDER BY id DESC LIMIT 1`, kind, collectionID)
}

func (s *Store) LatestJob(ctx context.Context, kind, collectionID string) (Job, bool, error) {
	return s.oneJob(ctx, `WHERE kind = ? AND collection_id = ? ORDER BY id DESC LIMIT 1`, kind, collectionID)
}

func (s *Store) oneJob(ctx context.Context, where string, args ...any) (Job, bool, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	return j, err == nil, err
}

func (s *Store) JobEvents(ctx context.Context, jobID int64) ([]JobEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT job_id, seq, at, level, message, record FROM job_events WHERE job_id = ? ORDER BY seq`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobEvent
	for rows.Next() {
		var (
			e                 JobEvent
			at, level, record string
		)
		if err := rows.Scan(&e.JobID, &e.Seq, &at, &level, &e.Message, &record); err != nil {
			return nil, err
		}
		if err := e.Level.UnmarshalText([]byte(level)); err != nil {
			return nil, err
		}
		e.Record = json.RawMessage(record)
		if e.At, err = parseJobTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var (
		j                                 Job
		params, result                    string
		created, started, finished, state string
	)
	if err := row.Scan(&j.ID, &j.Kind, &j.Trigger, &j.CollectionID, &j.ProfileID, &params, &state,
		&created, &started, &finished, &j.Error, &j.CostUSD, &result); err != nil {
		return Job{}, err
	}
	j.Status = JobStatus(state)
	j.Params = json.RawMessage(params)
	if result != "" {
		j.Result = json.RawMessage(result)
	}
	var err error
	if j.CreatedAt, err = parseJobTime(created); err != nil {
		return Job{}, err
	}
	if j.StartedAt, err = parseJobTime(started); err != nil {
		return Job{}, err
	}
	if j.FinishedAt, err = parseJobTime(finished); err != nil {
		return Job{}, err
	}
	return j, nil
}

func parseJobTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
