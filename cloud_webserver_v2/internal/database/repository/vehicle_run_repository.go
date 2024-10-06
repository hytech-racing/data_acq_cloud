package repository

import (
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/mongo"
)

type VehicleRunRepository interface {
	Save(vehicleRun models.VehicleModel) error
}

type MySQLOrderRepository struct {
	db *mongo.Client
}

func (repo *MySQLOrderRepository) Save(vehicleRun models.VehicleModel) error {
	return nil
}
