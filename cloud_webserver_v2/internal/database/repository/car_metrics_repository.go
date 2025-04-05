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

type CarMetricsRepository interface {
	Save(ctx context.Context, carMetrics models.CarMetricsModel) (models.CarMetricsModel, error)
	UpdateMetricByID(ctx context.Context, idStr string, metricUpdates models.CarMetricsModel) (models.CarMetricsModel, error)
	GetCarMetricsFromId(ctx context.Context, idStr string) (*models.CarMetricsModel, error)
}

type MongoCarMetricsRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

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

func (repo *MongoCarMetricsRepository) Save(ctx context.Context, metrics models.CarMetricsModel) (models.CarMetricsModel, error) {
	res, err := repo.collection.InsertOne(ctx, metrics)
	if err != nil {
		return models.CarMetricsModel{}, fmt.Errorf("could not insert car model data: %v", err)
	}

	metrics.Id = res.InsertedID.(primitive.ObjectID)
	return metrics, nil
}

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
