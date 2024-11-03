package models

import (
	"context"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FileModel struct {
	AwsBucket string `bson:"aws_bucket"`
	FilePath  string `bson:"file_path"`
	FileName  string `bson:"file_name"`
}

type FileModelResponse struct {
	SignedUrl string `json:"signed_url"`
	FileName  string `json:"file_name"`
}

type VehicleRunModel struct {
	Id             primitive.ObjectID     `bson:"_id,omitempty"`
	Date           time.Time              `bson:"date"`
	CarModel       string                 `bson:"car_model"`
	SchemaVersions map[string]string      `bson:"schema_versions"`
	Notes          *string                `bson:"notes,omitempty"`
	McapFiles      []FileModel            `bson:"mcap_files,omitempty"`
	MatFiles       []FileModel            `bson:"mat_files,omitempty"`
	ContentFiles   map[string][]FileModel `bson:"graph_files,omitempty"`
	Location       *string                `bson:"location,omitempty"`
	EventType      *string                `bson:"event_type,omitempty"`
	DynamicFields  map[string]interface{} `bson:"dynamic_fields,omitempty"`
}

type VehicleRunModelResponse struct {
	Id             primitive.ObjectID             `json:"id"`
	Date           time.Time                      `json:"date"`
	CarModel       string                         `json:"car_model"`
	SchemaVersions map[string]string              `json:"schema_versions"`
	Notes          *string                        `json:"notes,omitempty"`
	McapFiles      []FileModelResponse            `json:"mcap_files,omitempty"`
	MatFiles       []FileModelResponse            `json:"mat_files,omitempty"`
	ContentFiles   map[string][]FileModelResponse `json:"graph_files,omitempty"`
	Location       *string                        `json:"location,omitempty"`
	EventType      *string                        `json:"event_type,omitempty"`
	DynamicFields  map[string]interface{}         `json:"dynamic_fields,omitempty"`
}

func VehicleRunSerialize(ctx context.Context, s3Repo *s3.S3Repository, model VehicleRunModel) VehicleRunModelResponse {
	modelOut := VehicleRunModelResponse{
		Id:             model.Id,
		Date:           model.Date,
		CarModel:       model.CarModel,
		SchemaVersions: model.SchemaVersions,
		Notes:          model.Notes,
		Location:       model.Location,
		EventType:      model.EventType,
		DynamicFields:  model.DynamicFields,
	}

	if model.McapFiles != nil && len(model.McapFiles) > 0 {
		fileResponses := getFileModelResponse(ctx, s3Repo, model.McapFiles)
		modelOut.McapFiles = fileResponses
	}

	if model.MatFiles != nil && len(model.MatFiles) > 0 {
		fileResponses := getFileModelResponse(ctx, s3Repo, model.MatFiles)
		modelOut.MatFiles = fileResponses
	}

	for key, files := range model.ContentFiles {
		modelOut.ContentFiles = make(map[string][]FileModelResponse)
		if len(files) > 0 {
			fileResponses := getFileModelResponse(ctx, s3Repo, files)
			modelOut.ContentFiles[key] = fileResponses
		}
	}

	return modelOut
}

func getFileModelResponse(ctx context.Context, s3Repo *s3.S3Repository, files []FileModel) []FileModelResponse {
	s3Bucket := s3Repo.Bucket()
	outFiles := make([]FileModelResponse, len(files))
	for idx, file := range files {
		signedUrl := s3Repo.GetSignedUrl(ctx, s3Bucket, file.FilePath)
		outFiles[idx] = FileModelResponse{
			SignedUrl: signedUrl,
			FileName:  file.FileName,
		}
	}

	return outFiles
}

func NewVehicleModel() *VehicleRunModel {
	return &VehicleRunModel{}
}
