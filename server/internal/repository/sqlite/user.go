package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
)

type UserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, email, display_name, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Email, user.DisplayName, user.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	var createdAt string
	var displayName sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Email, &displayName, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	user.DisplayName = displayName.String
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &user, nil
}

func (r *UserRepository) FindByIDs(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	result := make(map[string]*domain.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT id, email, display_name, created_at FROM users WHERE id IN (` +
		strings.Join(placeholders, ",") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var user domain.User
		var createdAt string
		var displayName sql.NullString
		if err := rows.Scan(&user.ID, &user.Email, &displayName, &createdAt); err != nil {
			return nil, err
		}
		user.DisplayName = displayName.String
		user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		result[user.ID] = &user
	}
	return result, rows.Err()
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	var createdAt string
	var displayName sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &displayName, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	user.DisplayName = displayName.String
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &user, nil
}

func (r *UserRepository) FindAll(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, email, display_name, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var user domain.User
		var createdAt string
		var displayName sql.NullString

		if err := rows.Scan(&user.ID, &user.Email, &displayName, &createdAt); err != nil {
			return nil, err
		}

		user.DisplayName = displayName.String
		user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		users = append(users, &user)
	}

	return users, rows.Err()
}

func (r *UserRepository) UpdateDisplayName(ctx context.Context, id string, displayName string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET display_name = ? WHERE id = ?`,
		displayName, id,
	)
	return err
}
