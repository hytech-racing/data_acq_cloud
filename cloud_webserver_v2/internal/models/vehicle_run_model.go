package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FileModel struct {
	AwsBucket string `bson:"aws_bucket"`
	FilePath  string `bson:"file_path"`
	FileName  string `bson:"file_name"`
}

type VehicleRunModel struct {
	Id             primitive.ObjectID     `bson:"_id,omitempty"`
	Date           time.Time              `bson:"date"`
	CarModel       string                 `bson:"car_model"`
	SchemaVersions map[string]string      `bson:"schema_versions"`
	Notes          *string                `bson:"notes,omitempty"`
	McapFiles      []FileModel            `bson:"mcap_files,omitempty"`
	MatFiles       []FileModel            `bson:"mat_files,omitempty"`
	ContentFiles   []FileModel            `bson:"content_files,omitempty"`
	Location       *string                `bson:"location,omitempty"`
	EventType      *string                `bson:"event_type,omitempty"`
	DynamicFields  map[string]interface{} `bson:"dynamic_fields,omitempty"`
}

func NewVehicleModel() *VehicleRunModel {
	return &VehicleRunModel{}
}
