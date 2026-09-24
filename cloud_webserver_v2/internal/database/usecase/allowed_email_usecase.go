package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
)

// ErrInvalidEmail is returned when an email is malformed and cannot be registered
var ErrInvalidEmail = errors.New("invalid email address")

// AllowedEmailUseCase manages the subset of emails that are allowed to log in through CAS
type AllowedEmailUseCase struct {
	allowedEmailRepo repository.AllowedEmailRepository
}

func NewAllowedEmailUseCase(allowedEmailRepo repository.AllowedEmailRepository) *AllowedEmailUseCase {
	return &AllowedEmailUseCase{
		allowedEmailRepo,
	}
}

// IsEmailAllowed reports whether the given email is on the allowlist. The lookup normalizes
// the email, so it is case and whitespace insensitive.
func (uc *AllowedEmailUseCase) IsEmailAllowed(ctx context.Context, email string) (bool, error) {
	normalized := normalizeEmail(email)
	if normalized == "" {
		return false, nil
	}

	_, err := uc.allowedEmailRepo.GetByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, repository.ErrEmailNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to look up allowed email: %w", err)
	}

	return true, nil
}

// RegisterEmail stores a new email in the allowlist so that it can log in
func (uc *AllowedEmailUseCase) RegisterEmail(ctx context.Context, email string) (models.AllowedEmailModel, error) {
	normalized := normalizeEmail(email)
	if !isValidEmail(normalized) {
		return models.AllowedEmailModel{}, ErrInvalidEmail
	}

	id, err := newAllowedEmailID()
	if err != nil {
		return models.AllowedEmailModel{}, err
	}

	model := models.AllowedEmailModel{
		Id:        id,
		Email:     normalized,
		CreatedAt: time.Now().UTC(),
	}

	resModel, err := uc.allowedEmailRepo.Save(ctx, model)
	if err != nil {
		return models.AllowedEmailModel{}, err
	}

	return resModel, nil
}

// GetAllEmails returns every email that is currently allowed to log in
func (uc *AllowedEmailUseCase) GetAllEmails(ctx context.Context) ([]models.AllowedEmailModel, error) {
	return uc.allowedEmailRepo.GetAll(ctx)
}

// normalizeEmail trims whitespace and lowercases an email so allowlist storage and
// lookups are consistent regardless of how CAS or an administrator spells it
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// isValidEmail rejects malformed addresses so obviously wrong values never reach the allowlist
func isValidEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email
}

// newAllowedEmailID creates a random identifier for a newly registered email
func newAllowedEmailID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("could not generate allowed email id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
