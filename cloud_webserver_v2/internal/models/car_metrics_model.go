package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// CarMetricsModel contains
type CarMetricsModel struct {
	Id       primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	CarModel string             `json:"car_model" bson:"car_model,omitempty"`
	// TODO: Add Information we want to track, empty for now as we haven't decided
}
