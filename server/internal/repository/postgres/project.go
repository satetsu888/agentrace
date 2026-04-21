package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
)

type ProjectRepository struct {
	db *DB
}

func NewProjectRepository(db *DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(ctx context.Context, project *domain.Project) error {
	if project.ID == "" {
		project.ID = uuid.New().String()
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO projects (id, canonical_git_repository, created_at)
		 VALUES ($1, $2, $3)`,
		project.ID, project.CanonicalGitRepository, project.CreatedAt,
	)
	return err
}

func (r *ProjectRepository) FindByID(ctx context.Context, id string) (*domain.Project, error) {
	return r.scanProject(r.db.QueryRowContext(ctx,
		`SELECT id, canonical_git_repository, created_at
		 FROM projects WHERE id = $1`,
		id,
	))
}

func (r *ProjectRepository) FindByIDs(ctx context.Context, ids []string) (map[string]*domain.Project, error) {
	result := make(map[string]*domain.Project, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "$" + strconv.Itoa(i+1)
		args[i] = id
	}

	query := `SELECT id, canonical_git_repository, created_at FROM projects WHERE id IN (` +
		strings.Join(placeholders, ",") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		p, err := r.scanProjectFromRows(rows)
		if err != nil {
			return nil, err
		}
		result[p.ID] = p
	}
	return result, rows.Err()
}

func (r *ProjectRepository) FindByCanonicalGitRepository(ctx context.Context, canonicalGitRepo string) (*domain.Project, error) {
	return r.scanProject(r.db.QueryRowContext(ctx,
		`SELECT id, canonical_git_repository, created_at
		 FROM projects WHERE canonical_git_repository = $1`,
		canonicalGitRepo,
	))
}

func (r *ProjectRepository) FindOrCreateByCanonicalGitRepository(ctx context.Context, canonicalGitRepo string) (*domain.Project, error) {
	// First try to find existing project
	project, err := r.FindByCanonicalGitRepository(ctx, canonicalGitRepo)
	if err != nil {
		return nil, err
	}

	if project != nil {
		return project, nil
	}

	// Create new project
	newProject := &domain.Project{
		ID:                     uuid.New().String(),
		CanonicalGitRepository: canonicalGitRepo,
		CreatedAt:              time.Now(),
	}

	if err := r.Create(ctx, newProject); err != nil {
		// Handle race condition - another process may have created it
		existingProject, findErr := r.FindByCanonicalGitRepository(ctx, canonicalGitRepo)
		if findErr != nil {
			return nil, err // Return original error
		}
		if existingProject != nil {
			return existingProject, nil
		}
		return nil, err
	}

	return newProject, nil
}

func (r *ProjectRepository) FindAll(ctx context.Context, limit int, cursor string) ([]*domain.Project, string, error) {
	query := `SELECT id, canonical_git_repository, created_at
		 FROM projects`

	var args []any
	paramIdx := 1

	// Apply cursor filter
	if cursor != "" {
		cursorInfo := repository.DecodeCursor(cursor)
		if cursorInfo != nil {
			cursorTime, err := cursorInfo.ParseSortTime()
			if err == nil {
				query += ` WHERE (created_at < $1 OR (created_at = $2 AND id < $3))`
				args = append(args, cursorTime, cursorTime, cursorInfo.ID)
				paramIdx = 4
			}
		}
	}

	query += ` ORDER BY created_at DESC, id DESC`

	if limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, paramIdx)
		args = append(args, limit+1)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		project, err := r.scanProjectFromRows(rows)
		if err != nil {
			return nil, "", err
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	// Generate next cursor if there are more results
	var nextCursor string
	if limit > 0 && len(projects) > limit {
		projects = projects[:limit]
		lastItem := projects[limit-1]
		nextCursor = repository.EncodeCursor(lastItem.CreatedAt, lastItem.ID)
	}

	return projects, nextCursor, nil
}

func (r *ProjectRepository) GetDefaultProject(ctx context.Context) (*domain.Project, error) {
	return r.FindByID(ctx, domain.DefaultProjectID)
}

func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

func (r *ProjectRepository) HasRelatedData(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sessions WHERE project_id = $1
			UNION ALL
			SELECT 1 FROM plan_documents WHERE project_id = $1
			LIMIT 1
		)`, id).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *ProjectRepository) scanProject(row *sql.Row) (*domain.Project, error) {
	var project domain.Project
	var createdAt sql.NullTime

	err := row.Scan(&project.ID, &project.CanonicalGitRepository, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		project.CreatedAt = createdAt.Time
	}

	return &project, nil
}

func (r *ProjectRepository) scanProjectFromRows(rows *sql.Rows) (*domain.Project, error) {
	var project domain.Project
	var createdAt sql.NullTime

	err := rows.Scan(&project.ID, &project.CanonicalGitRepository, &createdAt)
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		project.CreatedAt = createdAt.Time
	}

	return &project, nil
}
