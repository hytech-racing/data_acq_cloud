package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type CarMetricsModel struct {
	Id       primitive.ObjectID `bson:"_id"`
	CarModel string             `bson:"car_model"`
}
