package repository

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const CarMetricsModel string = "car_metrics"

// CarMetricsRepository contains the methods any db implementation needs to implement to interact with car metrics data
type CarMetricsRepository interface {
	GetAllCarMetrics(ctx context.Context) ([]models.CarMetricsModel, error)
	Save(ctx context.Context, carMetrics models.CarMetricsModel) (models.CarMetricsModel, error)
	UpdateMetricByID(ctx context.Context, idStr string, metricUpdates models.CarMetricsModel) (models.CarMetricsModel, error)
	GetCarMetricsFromId(ctx context.Context, idStr string) (*models.CarMetricsModel, error)
}

// MongoCarMetricsRepository conatins all the information needed to interact with a MongoDB implementation of the CarMetrics db
type MongoCarMetricsRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

// NewMongoCarMetricsRepository creates a new NewMongoCarMetricsRepository with a MongoDB client and database
func NewMongoCarMetricsRepository(dbClient *mongo.Client, database *mongo.Database) (*MongoCarMetricsRepository, error) {
	collection := database.Collection(CarMetricsModel)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection %s", CarMetricsModel)
	}

	return &MongoCarMetricsRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

// Save creates a new Car Metrics document in the collection.
// For now, I see this being used rarely as the current plan is to create a new document for each car (HT08, Ht09, etc.)
// May change if we want to target more granular iterations of each car
func (repo *MongoCarMetricsRepository) Save(ctx context.Context, metrics models.CarMetricsModel) (models.CarMetricsModel, error) {
	res, err := repo.collection.InsertOne(ctx, metrics)
	if err != nil {
		return models.CarMetricsModel{}, fmt.Errorf("could not insert car model data: %v", err)
	}

	metrics.Id = res.InsertedID.(primitive.ObjectID)
	return metrics, nil
}

// UpdateMetricByID updates a document with idStr and updates the contents of the document with data specified in CarMetricsModel
func (repo *MongoCarMetricsRepository) UpdateMetricByID(ctx context.Context, idStr string, metricUpdates models.CarMetricsModel) (models.CarMetricsModel, error) {
	updatedMetrics := models.CarMetricsModel{}
	updateDoc, err := bson.Marshal(metricUpdates)
	if err != nil {
		return updatedMetrics, fmt.Errorf("failed to marshall bson for %v", metricUpdates)
	}

	var bsonDoc bson.M
	err = bson.Unmarshal(updateDoc, &bsonDoc)
	if err != nil {
		return updatedMetrics, fmt.Errorf("failed to create bson data for %v", metricUpdates)
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	objID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return updatedMetrics, fmt.Errorf("invalid id: %s", idStr)
	}
	filter := bson.M{"_id": objID}
	updateFunc := bson.M{"$set": bsonDoc}

	err = repo.collection.FindOneAndUpdate(ctx, filter, updateFunc, opts).Decode(&updatedMetrics)
	if err != nil {
		return updatedMetrics, fmt.Errorf("failed to update record in %s: %v", CarMetricsModel, err)
	}

	return updatedMetrics, nil
}

// GetCarMetricsFromId gets a document from the MongoDB document with its idStr
func (repo *MongoCarMetricsRepository) GetCarMetricsFromId(ctx context.Context, idStr string) (*models.CarMetricsModel, error) {
	objID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid id: %s", idStr)
	}
	filter := bson.M{"_id": objID}
	result := repo.collection.FindOne(ctx, filter)
	if result.Err() != nil {
		return nil, result.Err()
	}

	var model models.CarMetricsModel
	err = result.Decode(&model)
	if err != nil {
		return nil, fmt.Errorf("could not decode result into model: %v", err)
	}

	return &model, err
}

func (repo *MongoCarMetricsRepository) GetAllCarMetrics(ctx context.Context) ([]models.CarMetricsModel, error) {
	filter := bson.M{}
	cursor, err := repo.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var metrics []models.CarMetricsModel
	if err = cursor.All(ctx, &metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}
