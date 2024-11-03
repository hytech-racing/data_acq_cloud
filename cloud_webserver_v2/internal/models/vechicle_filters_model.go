package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type VehicleRunModelFilters struct {
	ID         *primitive.ObjectID `bson:"id",omitempty`
	BeforeDate *time.Time          `bson:"before_date,omitempty"`
	AfterDate  *time.Time          `bson:"after_date,omitempty"`
	Location   *string             `bson:"location",omitempty`
	EventType  *string             `bson:"event_type",omitempty`
	CarModel   *string             `bson:"car_model",omitempty`
	SearchText *string
}
