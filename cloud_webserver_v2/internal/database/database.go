package database

import (
	"context"
	"fmt"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/database/usecase"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// A DatabaseClient establishes a connection to the MongoDB database and allows
// for interfacing through the different collections through it.
// Whoever uses this struct to establish a connection to the database is responsible
// for calling the Disconnect() method to gracefully disconnect from the database
type DatabaseClient struct {
	databaseClient       *mongo.Client
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
	}

	vehicleDataDatabase := client.Database(VehicleDataDatabase)
	if vehicleDataDatabase == nil {
		return nil, fmt.Errorf("could not connect to database: %v", vehicleDataDatabase)
	}

	vehicleRunRepository, err := repository.NewMongoVehicleRunRepository(client, vehicleDataDatabase)
	if err != nil {
		return nil, fmt.Errorf("could not create vechicleRunRepository: %v", err)
	}
	databaseClient.vehicleRunRepository = vehicleRunRepository

	return databaseClient, nil
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
