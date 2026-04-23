package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"

)

type FoxgloveConfigsUseCase struct {
	foxgloveConfigsRepo repository.FoxgloveConfigRepository
}

func NewFoxgloveConfigsUseCase(foxgloveConfigsRepo repository.FoxgloveConfigRepository) *FoxgloveConfigsUseCase {
	return &FoxgloveConfigsUseCase{
		foxgloveConfigsRepo,
	}
}

func (uc *FoxgloveConfigsUseCase) CreateFoxgloveConfigs(ctx context.Context, model models.FoxgloveConfigModel) (models.FoxgloveConfigModel, error) {
	resModel, err := uc.foxgloveConfigsRepo.SaveFoxgloveConfig(ctx, model)
	if err != nil {
		return models.FoxgloveConfigModel{}, err
	}

	return resModel, nil
}

func (uc *FoxgloveConfigsUseCase) GetFoxgloveConfigFromId(ctx context.Context, id string) (*models.FoxgloveConfigModel, error) {
	return uc.foxgloveConfigsRepo.GetFoxgloveConfigFromId(ctx, id)
}

func (uc *FoxgloveConfigsUseCase) GetAllFoxgloveConfigs(ctx context.Context) ([]models.FoxgloveConfigModel, error) {
	return uc.foxgloveConfigsRepo.GetAllFoxgloveConfigs(ctx)
}

func (uc *FoxgloveConfigsUseCase) DeleteFoxgloveConfigFromId(ctx context.Context, id primitive.ObjectID) error {
	return uc.foxgloveConfigsRepo.DeleteFoxgloveConfigFromID(ctx, id)
}
