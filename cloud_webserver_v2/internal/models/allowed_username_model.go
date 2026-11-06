package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// AllowedUsernameModel is one username that is permitted to sign in through Georgia Tech SSO.
type AllowedUsernameModel struct {
	Id       primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Username string             `json:"username" bson:"username"`
}
