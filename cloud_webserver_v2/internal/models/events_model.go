package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Events: job_started, mcap_uploaded, hdf5_uploaded, plots_uploaded, job_completed, hdf5_failed, job_failed
type EventsModel struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty"`
	McapID       *primitive.ObjectID `bson:"mcap_id,omitempty"`
	Event        *string             `bson:"event,omitempty"`
	Error        *string             `bson:"error,omitempty"`
	CreatedAt    *time.Time          `bson:"created_at,omitempty"`
}
