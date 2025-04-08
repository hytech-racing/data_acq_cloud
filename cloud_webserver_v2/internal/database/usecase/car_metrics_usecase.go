package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
)

type CarMetricsUseCase struct {
	carMetricsRepo repository.CarMetricsRepository
}

func NewCarMetricsUseCase(carMetricsRepo repository.CarMetricsRepository) *CarMetricsUseCase {
	return &CarMetricsUseCase{
		carMetricsRepo,
	}
}

func (uc *CarMetricsUseCase) CreateCarMetrics(ctx context.Context, model models.CarMetricsModel) (models.CarMetricsModel, error) {
	resModel, err := uc.carMetricsRepo.Save(ctx, model)
	if err != nil {
		return models.CarMetricsModel{}, err
	}

	return resModel, nil
}

func (uc *CarMetricsUseCase) GetCarMetricsById(ctx context.Context, id string) (*models.CarMetricsModel, error) {
	return uc.carMetricsRepo.GetCarMetricsFromId(ctx, id)
}

func (uc *CarMetricsUseCase) GetAllCarMetrics(ctx context.Context) ([]models.CarMetricsModel, error) {
	return uc.carMetricsRepo.GetAllCarMetrics(ctx)
}

func (uc *CarMetricsUseCase) UpdateCarMetrics(ctx context.Context, id string, updates models.CarMetricsModel) (models.CarMetricsModel, error) {
	return uc.carMetricsRepo.UpdateMetricByID(ctx, id, updates)
}
