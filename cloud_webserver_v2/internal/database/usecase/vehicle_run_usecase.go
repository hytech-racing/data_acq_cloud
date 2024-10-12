package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
)

type VehicleRunUseCase struct {
	vechicleRunRepo *repository.MongoVehicleRunRepository
}

func NewVehicleRunUseCase(vehicleRunRepo *repository.MongoVehicleRunRepository) *VehicleRunUseCase {
	return &VehicleRunUseCase{
		vechicleRunRepo: vehicleRunRepo,
	}
}

func (uc *VehicleRunUseCase) CreateVehicleRun(ctx context.Context, model *models.VehicleRunModel) (*models.VehicleRunModel, error) {
	model, err := uc.vechicleRunRepo.Save(ctx, model)
	if err != nil {
		return nil, err
	}
	return model, nil
}
