package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const AllowedUsernameCollection string = "allowed_usernames"

// ErrUsernameAlreadyExists is returned by Add when the username is already on the allowlist.
var ErrUsernameAlreadyExists = errors.New("username already exists")

// AllowedUsernameRepository contains the methods needed to manage the login allowlist.
type AllowedUsernameRepository interface {
	GetAll(ctx context.Context) ([]models.AllowedUsernameModel, error)
	Contains(ctx context.Context, username string) (bool, error)
	Add(ctx context.Context, username string) error
	Remove(ctx context.Context, username string) error
}

// MongoAllowedUsernameRepository stores the login allowlist in MongoDB.
type MongoAllowedUsernameRepository struct {
	dbClient   *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection
}

// NewMongoAllowedUsernameRepository creates the allowlist repository and ensures the
// username field is unique so the same user cannot be added twice.
func NewMongoAllowedUsernameRepository(ctx context.Context, dbClient *mongo.Client, database *mongo.Database) (*MongoAllowedUsernameRepository, error) {
	collection := database.Collection(AllowedUsernameCollection)
	if collection == nil {
		return nil, fmt.Errorf("could not get collection %s", AllowedUsernameCollection)
	}

	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, fmt.Errorf("could not create unique index on %s.username: %w", AllowedUsernameCollection, err)
	}

	return &MongoAllowedUsernameRepository{
		dbClient:   dbClient,
		db:         database,
		collection: collection,
	}, nil
}

// GetAll returns every allowed username, sorted alphabetically.
func (repo *MongoAllowedUsernameRepository) GetAll(ctx context.Context) ([]models.AllowedUsernameModel, error) {
	cursor, err := repo.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "username", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("could not find allowed usernames: %w", err)
	}
	defer cursor.Close(ctx)
	var usernames []models.AllowedUsernameModel
	if err = cursor.All(ctx, &usernames); err != nil {
		return nil, fmt.Errorf("could not decode allowed usernames: %w", err)
	}
	if usernames == nil {
		usernames = make([]models.AllowedUsernameModel, 0)
	}
	return usernames, nil
}

// Contains reports whether username is on the allowlist.
func (repo *MongoAllowedUsernameRepository) Contains(ctx context.Context, username string) (bool, error) {
	count, err := repo.collection.CountDocuments(
		ctx,
		bson.M{"username": username},
		options.Count().SetLimit(1),
	)
	if err != nil {
		return false, fmt.Errorf("could not check allowed username: %w", err)
	}
	return count > 0, nil
}

// Add puts username on the allowlist.
func (repo *MongoAllowedUsernameRepository) Add(ctx context.Context, username string) error {
	_, err := repo.collection.InsertOne(ctx, models.AllowedUsernameModel{Username: username})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrUsernameAlreadyExists
		}
		return fmt.Errorf("could not insert allowed username: %w", err)
	}
	return nil
}

// Remove takes username off the allowlist. Removing a username that is not present is a no-op.
func (repo *MongoAllowedUsernameRepository) Remove(ctx context.Context, username string) error {
	_, err := repo.collection.DeleteOne(ctx, bson.M{"username": username})
	if err != nil {
		return fmt.Errorf("could not delete allowed username: %w", err)
	}
	return nil
}
