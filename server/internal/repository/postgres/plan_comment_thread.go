package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
)

type PlanCommentThreadRepository struct {
	db *DB
}

func NewPlanCommentThreadRepository(db *DB) *PlanCommentThreadRepository {
	return &PlanCommentThreadRepository{db: db}
}

func (r *PlanCommentThreadRepository) Create(ctx context.Context, thread *domain.PlanCommentThread) error {
	if thread.ID == "" {
		thread.ID = uuid.New().String()
	}
	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	if thread.UpdatedAt.IsZero() {
		thread.UpdatedAt = now
	}
	if thread.Status == "" {
		thread.Status = domain.PlanCommentThreadStatusActive
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO plan_comment_threads (id, plan_document_id, target_text, context_before, context_after, original_body_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		thread.ID, thread.PlanDocumentID,
		thread.TargetText, thread.ContextBefore, thread.ContextAfter, thread.OriginalBodyHash,
		string(thread.Status),
		thread.CreatedAt, thread.UpdatedAt,
	)
	return err
}

func (r *PlanCommentThreadRepository) FindByID(ctx context.Context, id string) (*domain.PlanCommentThread, error) {
	return r.scanThread(r.db.QueryRowContext(ctx,
		`SELECT id, plan_document_id, target_text, context_before, context_after, original_body_hash, status, created_at, updated_at
		 FROM plan_comment_threads WHERE id = $1`,
		id,
	))
}

func (r *PlanCommentThreadRepository) FindByPlanDocumentID(ctx context.Context, planDocumentID string, status *domain.PlanCommentThreadStatus) ([]*domain.PlanCommentThread, error) {
	query := `SELECT id, plan_document_id, target_text, context_before, context_after, original_body_hash, status, created_at, updated_at
		 FROM plan_comment_threads WHERE plan_document_id = $1`
	args := []any{planDocumentID}

	if status != nil {
		query += " AND status = $2"
		args = append(args, string(*status))
	}

	query += " ORDER BY created_at ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []*domain.PlanCommentThread
	for rows.Next() {
		thread, err := r.scanThreadFromRows(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return threads, nil
}

func (r *PlanCommentThreadRepository) CountActiveByPlanDocumentID(ctx context.Context, planDocumentID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plan_comment_threads WHERE plan_document_id = $1 AND status = $2`,
		planDocumentID, string(domain.PlanCommentThreadStatusActive),
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PlanCommentThreadRepository) Update(ctx context.Context, thread *domain.PlanCommentThread) error {
	thread.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx,
		`UPDATE plan_comment_threads SET status = $1, updated_at = $2
		 WHERE id = $3`,
		string(thread.Status), thread.UpdatedAt, thread.ID,
	)
	return err
}

func (r *PlanCommentThreadRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM plan_comment_threads WHERE id = $1`,
		id,
	)
	return err
}

func (r *PlanCommentThreadRepository) MarkOutdatedByPlanDocumentID(ctx context.Context, planDocumentID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE plan_comment_threads SET status = $1, updated_at = $2
		 WHERE plan_document_id = $3 AND status = $4`,
		string(domain.PlanCommentThreadStatusOutdated), time.Now(),
		planDocumentID, string(domain.PlanCommentThreadStatusActive),
	)
	return err
}

func (r *PlanCommentThreadRepository) scanThread(row *sql.Row) (*domain.PlanCommentThread, error) {
	var thread domain.PlanCommentThread
	var status string
	var createdAt, updatedAt sql.NullTime

	err := row.Scan(
		&thread.ID, &thread.PlanDocumentID,
		&thread.TargetText, &thread.ContextBefore, &thread.ContextAfter, &thread.OriginalBodyHash,
		&status, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	thread.Status = domain.PlanCommentThreadStatus(status)
	if createdAt.Valid {
		thread.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		thread.UpdatedAt = updatedAt.Time
	}

	return &thread, nil
}

func (r *PlanCommentThreadRepository) scanThreadFromRows(rows *sql.Rows) (*domain.PlanCommentThread, error) {
	var thread domain.PlanCommentThread
	var status string
	var createdAt, updatedAt sql.NullTime

	err := rows.Scan(
		&thread.ID, &thread.PlanDocumentID,
		&thread.TargetText, &thread.ContextBefore, &thread.ContextAfter, &thread.OriginalBodyHash,
		&status, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	thread.Status = domain.PlanCommentThreadStatus(status)
	if createdAt.Valid {
		thread.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		thread.UpdatedAt = updatedAt.Time
	}

	return &thread, nil
}
