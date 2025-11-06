package repository

import (
	"context"

	"magnet-player/internal/domain"
)

// UserRepository defines persistence operations for User entities.
type UserRepository interface {
	Init(ctx context.Context) error
	Create(ctx context.Context, user *domain.User) (int64, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}
