package repository

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const foxgloveConfigFileModel string = "foxglove_configs"

// FoxgloveConfigRepository contains the methods any db implementation needs to implement to interact with foxglove configs
type FoxgloveConfigRepository interface {
	GetAllFoxgloveConfigs(ctx context.Context) ([]models.FoxgloveConfigModel, error)
	SaveFoxgloveConfig(ctx context.Context, foxgloveconfig models.FoxgloveConfigModel) (models.FoxgloveConfigModel, error)
	GetFoxgloveConfigFromId(ctx context.Context, idStr string) (*models.FoxgloveConfigModel, error)
	DeleteFoxgloveConfigFromID(ctx context.Context, id primitive.ObjectID) error
}

// MongoFoxgloveConfigRepository conatins all the information needed to interact with a MongoDB implementation of the FoxgloveConfigs db
type MongoFoxgloveConfigRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

// MongoFoxgloveConfigRepository creates a new NewMongoFoxgloveConfigRepository with a MongoDB client and database
func NewMongoFoxgloveConfigRepository(dbClient *mongo.Client, database *mongo.Database) (*MongoFoxgloveConfigRepository, error) {
	collection := database.Collection(foxgloveConfigFileModel)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection %s", foxgloveConfigFileModel)
	}

	return &MongoFoxgloveConfigRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

// SaveFoxgloveConfig creates a new Foxglove Config document in the collection.
func (repo *MongoFoxgloveConfigRepository) SaveFoxgloveConfig(ctx context.Context, config models.FoxgloveConfigModel) (models.FoxgloveConfigModel, error) {
	res, err := repo.collection.InsertOne(ctx, config)
	if err != nil {
		return models.FoxgloveConfigModel{}, fmt.Errorf("could not insert foxglove config: %v", err)
	}

	config.Id = res.InsertedID.(primitive.ObjectID)
	return config, nil
}

// GetFoxgloveConfigFromId gets a document from the MongoDB document with its idStr
func (repo *MongoFoxgloveConfigRepository) GetFoxgloveConfigFromId(ctx context.Context, idStr string) (*models.FoxgloveConfigModel, error) {
	objID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %s", idStr)
	}
	filter := bson.M{"_id": objID}
	result := repo.collection.FindOne(ctx, filter)
	if result.Err() != nil {
		return nil, result.Err()
	}

	var model models.FoxgloveConfigModel
	err = result.Decode(&model)
	if err != nil {
		return nil, fmt.Errorf("could not decode result into model: %v", err)
	}

	return &model, err
}

func (repo *MongoFoxgloveConfigRepository) GetAllFoxgloveConfigs(ctx context.Context) ([]models.FoxgloveConfigModel, error) {
	filter := bson.M{}
	cursor, err := repo.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var config []models.FoxgloveConfigModel
	if err = cursor.All(ctx, &config); err != nil {
		return nil, err
	}

	return config, nil
}

// Delete a FoxgloveConfigModel from the MongoDB database from a VehicleRun ID
func (repo *MongoFoxgloveConfigRepository) DeleteFoxgloveConfigFromID(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	_, err := repo.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}