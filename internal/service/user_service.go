package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"magnet-player/internal/domain"
	"magnet-player/internal/repository"
)

var (
	// ErrInvalidCredentials indicates that provided login credentials are incorrect.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidRegistrationPassword indicates the registration secret is incorrect.
	ErrInvalidRegistrationPassword = errors.New("invalid registration password")
	// ErrUserAlreadyExists is returned when attempting to register with an existing username.
	ErrUserAlreadyExists = errors.New("user already exists")
)

// UserService describes user lifecycle operations.
type UserService interface {
	Register(ctx context.Context, username, password, providedSecret string) (*domain.User, error)
	Authenticate(ctx context.Context, username, password string) (*domain.User, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
}

type userService struct {
	users          repository.UserRepository
	registerSecret string
}

func NewUserService(users repository.UserRepository, registerSecret string) UserService {
	return &userService{
		users:          users,
		registerSecret: strings.TrimSpace(registerSecret),
	}
}

func (s *userService) Register(ctx context.Context, username, password, providedSecret string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	providedSecret = strings.TrimSpace(providedSecret)
	password = strings.TrimSpace(password)

	if username == "" {
		return nil, errors.New("username is required")
	}
	if password == "" {
		return nil, errors.New("password is required")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	if s.registerSecret == "" {
		return nil, fmt.Errorf("registration secret is not configured")
	}
	if subtle.ConstantTimeCompare([]byte(providedSecret), []byte(s.registerSecret)) != 1 {
		return nil, ErrInvalidRegistrationPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if _, err := s.users.Create(ctx, user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil, ErrUserAlreadyExists
		}
		return nil, err
	}

	return sanitizeUser(user), nil
}

func (s *userService) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return sanitizeUser(user), nil
}

func (s *userService) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return sanitizeUser(user), nil
}

func sanitizeUser(user *domain.User) *domain.User {
	if user == nil {
		return nil
	}
	return &domain.User{
		ID:        user.ID,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
