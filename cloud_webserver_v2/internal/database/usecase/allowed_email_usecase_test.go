package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
)

// fakeAllowedEmailRepository is an in-memory AllowedEmailRepository used to test the
// use case without touching the file system.
type fakeAllowedEmailRepository struct {
	emails    map[string]models.AllowedEmailModel
	lookupErr error
}

func newFakeAllowedEmailRepository() *fakeAllowedEmailRepository {
	return &fakeAllowedEmailRepository{emails: make(map[string]models.AllowedEmailModel)}
}

func (repo *fakeAllowedEmailRepository) GetByEmail(_ context.Context, email string) (*models.AllowedEmailModel, error) {
	if repo.lookupErr != nil {
		return nil, repo.lookupErr
	}
	model, ok := repo.emails[email]
	if !ok {
		return nil, repository.ErrEmailNotFound
	}
	return &model, nil
}

func (repo *fakeAllowedEmailRepository) GetAll(_ context.Context) ([]models.AllowedEmailModel, error) {
	emails := make([]models.AllowedEmailModel, 0, len(repo.emails))
	for _, model := range repo.emails {
		emails = append(emails, model)
	}
	return emails, nil
}

func (repo *fakeAllowedEmailRepository) Save(_ context.Context, allowedEmail models.AllowedEmailModel) (models.AllowedEmailModel, error) {
	if _, ok := repo.emails[allowedEmail.Email]; ok {
		return models.AllowedEmailModel{}, repository.ErrEmailAlreadyRegistered
	}
	repo.emails[allowedEmail.Email] = allowedEmail
	return allowedEmail, nil
}

func TestIsEmailAllowedNormalizesEmail(t *testing.T) {
	repo := newFakeAllowedEmailRepository()
	repo.emails["george.burdell@gatech.edu"] = models.AllowedEmailModel{Email: "george.burdell@gatech.edu"}
	uc := NewAllowedEmailUseCase(repo)

	allowed, err := uc.IsEmailAllowed(context.Background(), "  George.Burdell@GaTech.edu ")
	if err != nil {
		t.Fatalf("IsEmailAllowed() error = %v", err)
	}
	if !allowed {
		t.Fatal("IsEmailAllowed() = false, want true for a registered email regardless of case")
	}
}

func TestIsEmailAllowedUnknownEmail(t *testing.T) {
	uc := NewAllowedEmailUseCase(newFakeAllowedEmailRepository())

	allowed, err := uc.IsEmailAllowed(context.Background(), "nobody@gatech.edu")
	if err != nil {
		t.Fatalf("IsEmailAllowed() error = %v", err)
	}
	if allowed {
		t.Fatal("IsEmailAllowed() = true, want false for an unregistered email")
	}
}

func TestIsEmailAllowedPropagatesLookupFailure(t *testing.T) {
	repo := newFakeAllowedEmailRepository()
	repo.lookupErr = errors.New("disk exploded")
	uc := NewAllowedEmailUseCase(repo)

	if _, err := uc.IsEmailAllowed(context.Background(), "george.burdell@gatech.edu"); err == nil {
		t.Fatal("IsEmailAllowed() error = nil, want the underlying lookup failure")
	}
}

func TestRegisterEmailGeneratesIDAndNormalizes(t *testing.T) {
	uc := NewAllowedEmailUseCase(newFakeAllowedEmailRepository())

	model, err := uc.RegisterEmail(context.Background(), "  George.Burdell@GaTech.edu ")
	if err != nil {
		t.Fatalf("RegisterEmail() error = %v", err)
	}
	if model.Email != "george.burdell@gatech.edu" {
		t.Errorf("RegisterEmail() email = %q, want the normalized address", model.Email)
	}
	if strings.TrimSpace(model.Id) == "" {
		t.Error("RegisterEmail() did not assign an id")
	}
	if model.CreatedAt.IsZero() {
		t.Error("RegisterEmail() did not assign a creation timestamp")
	}
}

func TestRegisterEmailRejectsInvalidEmail(t *testing.T) {
	uc := NewAllowedEmailUseCase(newFakeAllowedEmailRepository())

	if _, err := uc.RegisterEmail(context.Background(), "not-an-email"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("RegisterEmail() error = %v, want %v", err, ErrInvalidEmail)
	}
}

func TestRegisterEmailPropagatesDuplicate(t *testing.T) {
	uc := NewAllowedEmailUseCase(newFakeAllowedEmailRepository())
	ctx := context.Background()

	if _, err := uc.RegisterEmail(ctx, "george.burdell@gatech.edu"); err != nil {
		t.Fatalf("first RegisterEmail() error = %v", err)
	}

	if _, err := uc.RegisterEmail(ctx, "GEORGE.BURDELL@gatech.edu"); !errors.Is(err, repository.ErrEmailAlreadyRegistered) {
		t.Fatalf("second RegisterEmail() error = %v, want %v", err, repository.ErrEmailAlreadyRegistered)
	}
}
