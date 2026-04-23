package repository

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const EventsCollection string = "events"

type EventsRepository interface {
	Save(ctx context.Context, events *models.EventsModel) (*models.EventsModel, error)
	GetWithEventsFilters(ctx context.Context, filters *bson.M) ([]models.EventsModel, error)
	GetEventsFromId(ctx context.Context, id primitive.ObjectID) (*models.EventsModel, error)
	DeleteEventsFromId(ctx context.Context, id primitive.ObjectID) error
	UpdateEventsFromId(ctx context.Context, id primitive.ObjectID, events *models.EventsModel) error
}

type MongoEventsRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

func NewMongoEventsRepository(dbClient *mongo.Client, database *mongo.Database) (*MongoEventsRepository, error) {
	collection := database.Collection(EventsCollection)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection %s", EventsCollection)
	}

	return &MongoEventsRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

// Inserts a EventsModel into the MongoDB database
func (repo *MongoEventsRepository) Save(ctx context.Context, events *models.EventsModel) (*models.EventsModel, error) {
	res, err := repo.collection.InsertOne(ctx, events)
	if err != nil {
		return nil, fmt.Errorf("could not insert events data: %v, received error: %v", events, err)
	}

	events.ID = res.InsertedID.(primitive.ObjectID)
	return events, nil
}

// Get a EventsModel from the MongoDB database with filters
func (repo *MongoEventsRepository) GetWithEventsFilters(ctx context.Context, filters *bson.M) ([]models.EventsModel, error) {
	cursor, err := repo.collection.Find(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("could not find in events data with filters %v, received error: %v", filters, err)
	}

	var modelResults []models.EventsModel

	if err = cursor.All(ctx, &modelResults); err != nil {
		return nil, err
	}

	if modelResults == nil {
		modelResults = make([]models.EventsModel, 0)
	}

	return modelResults, nil
}

// Get a EventsModel from the MongoDB database from a Events ID
func (repo *MongoEventsRepository) GetEventsFromId(ctx context.Context, id primitive.ObjectID) (*models.EventsModel, error) {
	filter := bson.M{"_id": id}
	result := repo.collection.FindOne(ctx, filter)
	if result.Err() != nil {
		return nil, result.Err()
	}

	var model models.EventsModel
	err := result.Decode(&model)
	if err != nil {
		return nil, fmt.Errorf("could not decode result into model: %v", err)
	}

	return &model, nil
}

// Delete a EventsModel from the MongoDB database from a Events ID
func (repo *MongoEventsRepository) DeleteEventsFromId(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	_, err := repo.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

// Updates a EventsModel from the MongoDB database from a Events ID and given Events
func (repo *MongoEventsRepository) UpdateEventsFromId(ctx context.Context, id primitive.ObjectID, events *models.EventsModel) error {
	filter := bson.M{"_id": id}
	resp := repo.collection.FindOneAndReplace(ctx, filter, events)
	if resp.Err() != nil {
		return resp.Err()
	}
	return nil
}
