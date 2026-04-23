package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// VehicleRunModelFilters contians all the possible ways to filter and query
// for a VehicleRun.
type EventsModelFilters struct {
	ID         *primitive.ObjectID `bson:"id",omitempty`
	BeforeDate *time.Time          `bson:"before_date,omitempty"`
	AfterDate  *time.Time          `bson:"after_date,omitempty"`
	EventType  *string             `bson:"event_type",omitempty`
}
