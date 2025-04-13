package repository

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const VehicleRunCollection string = "vehicle_run"

type VehicleRunRepository interface {
	Save(ctx context.Context, vehicleRun *models.VehicleRunModel) (*models.VehicleRunModel, error)
	GetWithVehicleFilters(ctx context.Context, filters *bson.M) ([]models.VehicleRunModel, error)
	GetVehicleRunFromId(ctx context.Context, id primitive.ObjectID) (*models.VehicleRunModel, error)
	DeleteVehicleRunFromId(ctx context.Context, id primitive.ObjectID) error
	UpdateVehicleRunFromId(ctx context.Context, id primitive.ObjectID, vehicleRun *models.VehicleRunModel) error
}

type MongoVehicleRunRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

func NewMongoVehicleRunRepository(dbClient *mongo.Client, database *mongo.Database) (*MongoVehicleRunRepository, error) {
	collection := database.Collection(VehicleRunCollection)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection %s", VehicleRunCollection)
	}

	return &MongoVehicleRunRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

// Inserts a VehicleRunModel into the MongoDB database
func (repo *MongoVehicleRunRepository) Save(ctx context.Context, vehicleRun *models.VehicleRunModel) (*models.VehicleRunModel, error) {
	res, err := repo.collection.InsertOne(ctx, vehicleRun)
	if err != nil {
		return nil, fmt.Errorf("could not insert vehicle run data: %v, received error: %v", vehicleRun, err)
	}

	vehicleRun.Id = res.InsertedID.(primitive.ObjectID)
	return vehicleRun, nil
}

// Get a VehicleRunModel from the MongoDB database with filters
func (repo *MongoVehicleRunRepository) GetWithVehicleFilters(ctx context.Context, filters *bson.M) ([]models.VehicleRunModel, error) {
	cursor, err := repo.collection.Find(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("could not find in vehicle run data with filters %v, received error: %v", filters, err)
	}

	var modelResults []models.VehicleRunModel

	if err = cursor.All(ctx, &modelResults); err != nil {
		return nil, err
	}

	if modelResults == nil {
		modelResults = make([]models.VehicleRunModel, 0)
	}

	return modelResults, nil
}

// Get a VehicleRunModel from the MongoDB database from a VehicleRun ID
func (repo *MongoVehicleRunRepository) GetVehicleRunFromId(ctx context.Context, id primitive.ObjectID) (*models.VehicleRunModel, error) {
	filter := bson.M{"_id": id}
	result := repo.collection.FindOne(ctx, filter)
	if result.Err() != nil {
		return nil, result.Err()
	}

	var model models.VehicleRunModel
	err := result.Decode(&model)
	if err != nil {
		return nil, fmt.Errorf("could not decode result into model: %v", err)
	}

	return &model, nil
}

// Delete a VehicleRunModel from the MongoDB database from a VehicleRun ID
func (repo *MongoVehicleRunRepository) DeleteVehicleRunFromId(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	_, err := repo.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

// Updates a VehicleRunModel from the MongoDB database from a VehicleRun ID and given vehicleRun
func (repo *MongoVehicleRunRepository) UpdateVehicleRunFromId(ctx context.Context, id primitive.ObjectID, vehicleRun *models.VehicleRunModel) error {
	filter := bson.M{"_id": id}
	resp := repo.collection.FindOneAndReplace(ctx, filter, vehicleRun)
	if resp.Err() != nil {
		return resp.Err()
	}
	return nil
}
