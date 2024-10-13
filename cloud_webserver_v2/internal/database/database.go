package database

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database/usecase"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DatabaseClient struct {
	databaseClient       *mongo.Client
	databases            map[string]*mongo.Database
	vehicleRunRepository *repository.MongoVehicleRunRepository
}

const VehicleDataDatabase = "vehicle_data_db"

func NewDatabaseClient(ctx context.Context, uri string) (*DatabaseClient, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		fmt.Println("Error pinging MongoDB:", err)
		return nil, err
	}

	databaseClient := &DatabaseClient{
		databaseClient: client,
		databases:      make(map[string]*mongo.Database),
	}

	mainDatabase := databaseClient.AddDatabase(client, VehicleDataDatabase)
	if mainDatabase == nil {
		return nil, fmt.Errorf("could not connect to database: %v", mainDatabase)
	}

	vehicleRunRepository, err := repository.NewMongoVehicleRunRepository(client, mainDatabase)
	if err != nil {
		return nil, fmt.Errorf("could not create vechicleRunRepository: %v", err)
	}
	databaseClient.vehicleRunRepository = vehicleRunRepository

	return databaseClient, nil
}

func (client *DatabaseClient) AddDatabase(mongoClient *mongo.Client, databaseName string) *mongo.Database {
	db, ok := client.databases[databaseName]
	if ok {
		return db
	}

	newDatabase := client.databaseClient.Database(databaseName)

	if newDatabase != nil {
		client.databases[databaseName] = newDatabase
	}

	return newDatabase
}

func (client *DatabaseClient) VehicleRunUseCase() *usecase.VehicleRunUseCase {
	return usecase.NewVehicleRunUseCase(client.vehicleRunRepository)
}

func (client *DatabaseClient) Disonnect(ctx context.Context) error {
	err := client.databaseClient.Disconnect(ctx)
	if err != nil {
		return fmt.Errorf("failed to disconnect MongoDB client: %w", err)
	}
	return nil
}
