package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (uc *VehicleRunUseCase) GetVehicleRunByFilters(ctx context.Context, filters *models.VehicleRunModelFilters) ([]models.VehicleRunModel, error) {
	bson_filters_m := bson.M{}
	bson_or := bson.A{}

	if filters.ID != nil {
		id, err := primitive.ObjectIDFromHex(filters.ID.Hex())
		if err != nil {
			return nil, err
		}
		bson_filters_m["id"] = id
	}

	if filters.BeforeDate != nil || filters.AfterDate != nil {
		dateFilter := bson.M{}
		if filters.BeforeDate != nil {
			dateFilter["$gte"] = *filters.BeforeDate
		}
		if filters.AfterDate != nil {
			dateFilter["$lte"] = *filters.AfterDate
		}
		bson_filters_m["date"] = dateFilter
	}

	if filters.CarModel != nil {
		bson_filters_m["car_model"] = bson.M{
			"$regex": primitive.Regex{Pattern: *filters.CarModel, Options: "i"},
		}
	}

	if filters.SearchText != nil {
		bson_or = append(bson_or, bson.M{
			"mcap_files.file_name": bson.M{
				"$regex": primitive.Regex{Pattern: *filters.SearchText, Options: "i"},
			},
		})
		bson_or = append(bson_or, bson.M{
			"mat_files.file_name": bson.M{
				"$regex": primitive.Regex{Pattern: *filters.SearchText, Options: "i"},
			},
		})
		bson_or = append(bson_or, bson.M{
			"content_files.file_name": bson.M{
				"$regex": primitive.Regex{Pattern: *filters.SearchText, Options: "i"},
			},
		})
		bson_or = append(bson_or, bson.M{
			"notes": bson.M{
				"$regex": primitive.Regex{Pattern: *filters.SearchText, Options: "i"},
			},
		})
	}

	if filters.Location != nil {
		bson_filters_m["location"] = bson.M{"$regex": primitive.Regex{Pattern: *filters.Location, Options: "i"}}
	}

	if filters.EventType != nil {
		bson_filters_m["event_type"] = bson.M{"$regex": primitive.Regex{Pattern: *filters.EventType, Options: "i"}}
	}

	if len(bson_or) != 0 {
		bson_filters_m["$or"] = bson_or
	}

	// Execute the query
	result, err := uc.vechicleRunRepo.GetWithVehicleFilters(context.TODO(), &bson_filters_m)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (uc *VehicleRunUseCase) GetVehicleRunById(ctx context.Context, id primitive.ObjectID) (*models.VehicleRunModel, error) {
	return uc.vechicleRunRepo.GetVehicleRunFromId(ctx, id)
}

func (uc *VehicleRunUseCase) DeleteVehicleRunById(ctx context.Context, id primitive.ObjectID) error {
	return uc.vechicleRunRepo.DeleteVehicleRunFromId(ctx, id)
}
