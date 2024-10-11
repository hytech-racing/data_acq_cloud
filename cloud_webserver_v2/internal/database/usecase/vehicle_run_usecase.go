package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
)

type VehicleRunUseCase struct {
	vechicleRunRepo repository.VehicleRunRepository
}

func newVehicleRunUseCase(vehicleRunRepo repository.VehicleRunRepository) *VehicleRunUseCase {
	return &VehicleRunUseCase{
		vechicleRunRepo: vehicleRunRepo,
	}
}

func (uc *VehicleRunUseCase) CreateVehicleRun(ctx context.Context, model *models.VehicleRunModel) error {
	uc.vechicleRunRepo.Save(ctx, model)
	return nil
}
