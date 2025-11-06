package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"magnet-player/internal/domain"
	"magnet-player/internal/repository"
)

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
`

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) repository.UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Init(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, createUsersTable); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}
	return nil
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) (int64, error) {
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	res, err := r.db.ExecContext(ctx, `
INSERT INTO users (username, password_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)`,
		user.Username,
		user.PasswordHash,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return 0, fmt.Errorf("user already exists: %w", err)
		}
		return 0, fmt.Errorf("insert user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("user last insert id: %w", err)
	}
	user.ID = id
	return id, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, created_at, updated_at
FROM users
WHERE username = ?`,
		username,
	)
	return scanUser(row)
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, created_at, updated_at
FROM users
WHERE id = ?`,
		id,
	)
	return scanUser(row)
}

func scanUser(row interface {
	Scan(dest ...any) error
}) (*domain.User, error) {
	var user domain.User
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &user, nil
}
