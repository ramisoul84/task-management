package service

import (
	"context"
	"strings"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/repository"
	"github.com/ramisoul84/task-management/pkg/auth"
	"github.com/ramisoul84/task-management/pkg/logger"
)

type UserService interface {
	Me(ctx context.Context, userID string) (*domain.User, error)
	UpdateProfile(ctx context.Context, userID string, req *domain.UpdateProfileRequest) (*domain.User, error)
	ChangePassword(ctx context.Context, userID string, req *domain.ChangePasswordRequest) error
}

type userService struct {
	users  repository.UserRepository
	hasher auth.Hasher
}

func NewUserService(users repository.UserRepository, hasher auth.Hasher) UserService {
	return &userService{users: users, hasher: hasher}
}

func (s *userService) Me(ctx context.Context, userID string) (*domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *userService) UpdateProfile(ctx context.Context, userID string, req *domain.UpdateProfileRequest) (*domain.User, error) {
	if err := s.users.UpdateName(ctx, userID, strings.TrimSpace(req.Name)); err != nil {
		return nil, err
	}
	return s.users.GetByID(ctx, userID)
}

// ChangePassword requires the current password, so a stolen access token
// alone cannot be used to take over the account.
func (s *userService) ChangePassword(ctx context.Context, userID string, req *domain.ChangePasswordRequest) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.hasher.Compare(user.PasswordHash, req.CurrentPassword); err != nil {
		return domain.ErrInvalidCredentials.WithMessage("current password is incorrect")
	}

	hash, err := s.hasher.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return err
	}

	logger.FromContext(ctx).Info().Str("user_id", userID).Msg("password changed")
	return nil
}
