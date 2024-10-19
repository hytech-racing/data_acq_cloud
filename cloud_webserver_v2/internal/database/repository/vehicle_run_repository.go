package repository

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const VehicleRunCollection string = "vehicle_run"

type VehicleRunRepository interface {
	Save(ctx context.Context, vehicleRun *models.VehicleRunModel) error
}

type MongoVehicleRunRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

func NewMongoVehicleRunRepository(dbClient *mongo.Client, database *mongo.Database) (*MongoVehicleRunRepository, error) {
	collection := database.Collection(VehicleRunCollection)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection: %v", VehicleRunCollection)
	}

	return &MongoVehicleRunRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

func (repo *MongoVehicleRunRepository) Save(ctx context.Context, vehicleRun *models.VehicleRunModel) (*models.VehicleRunModel, error) {
	res, err := repo.collection.InsertOne(ctx, vehicleRun)
	if err != nil {
		return nil, fmt.Errorf("could not insert vehicle run data: %v, received error: %v", vehicleRun, err)
	}

	vehicleRun.Id = res.InsertedID.(primitive.ObjectID)

	return vehicleRun, nil
}
