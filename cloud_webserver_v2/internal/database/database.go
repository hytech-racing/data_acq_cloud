package database

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DatabaseClient struct {
	databaseClient *mongo.Client
	databases      map[string]*mongo.Database
}

func NewDatabaseClient(ctx context.Context, uri string) *DatabaseClient {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}

	return &DatabaseClient{
		databaseClient: client,
		databases:      make(map[string]*mongo.Database),
	}
}

func (client *DatabaseClient) NewDatabase(mongoClient *mongo.Client, databaseName string) {
	newDatabase := client.databaseClient.Database(databaseName)
	if newDatabase != nil {
		client.databases[databaseName] = newDatabase
	}
}
