package usecase

import (
	"context"
	"errors"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
)

type AllowedUsernameUseCase struct {
	allowedUsernameRepo repository.AllowedUsernameRepository
}

func NewAllowedUsernameUseCase(allowedUsernameRepo repository.AllowedUsernameRepository) *AllowedUsernameUseCase {
	return &AllowedUsernameUseCase{
		allowedUsernameRepo,
	}
}

func (uc *AllowedUsernameUseCase) GetAllUsernames(ctx context.Context) ([]string, error) {
	records, err := uc.allowedUsernameRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	usernames := make([]string, 0, len(records))
	for _, record := range records {
		usernames = append(usernames, record.Username)
	}
	return usernames, nil
}

func (uc *AllowedUsernameUseCase) ContainsUsername(ctx context.Context, username string) (bool, error) {
	return uc.allowedUsernameRepo.Contains(ctx, username)
}

func (uc *AllowedUsernameUseCase) AddUsername(ctx context.Context, username string) error {
	err := uc.allowedUsernameRepo.Add(ctx, username)
	if errors.Is(err, repository.ErrUsernameAlreadyExists) {
		return errors.New("username already exists")
	}
	return err
}

func (uc *AllowedUsernameUseCase) RemoveUsername(ctx context.Context, username string) error {
	return uc.allowedUsernameRepo.Remove(ctx, username)
}

// EnsureDefaultUsername adds username when the allowlist is empty, so that a fresh
// deployment always has at least one account that can sign in.
func (uc *AllowedUsernameUseCase) EnsureDefaultUsername(ctx context.Context, username string) error {
	if username == "" {
		return nil
	}
	existing, err := uc.allowedUsernameRepo.GetAll(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	err = uc.allowedUsernameRepo.Add(ctx, username)
	if err != nil && !errors.Is(err, repository.ErrUsernameAlreadyExists) {
		return err
	}
	return nil
}
