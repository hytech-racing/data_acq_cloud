package models

import (
	"context"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FileModel contains the information for a file (object) stored on S3
type FileModel struct {
	AwsBucket string `bson:"aws_bucket"`
	FilePath  string `bson:"file_path"`
	FileName  string `bson:"file_name"`
}

// FileModel contains the information for a serialized response of a file (object) stored on S3
type FileModelResponse struct {
	SignedUrl string `json:"signed_url"`
	FileName  string `json:"file_name"`
}

type VehicleRunModel struct {
	Id             primitive.ObjectID     `bson:"_id,omitempty"`
	ContentFiles   map[string][]FileModel `bson:"content_files,omitempty"`
	SchemaVersions map[string]string      `bson:"schema_versions"`
	Notes          *string                `bson:"notes,omitempty"`
	Location       *string                `bson:"location,omitempty"`
	EventType      *string                `bson:"event_type,omitempty"`
	DynamicFields  map[string]interface{} `bson:"dynamic_fields,omitempty"`
	McapFiles      []FileModel            `bson:"mcap_files,omitempty"`
	CarModel       string                 `bson:"car_model"`
	Date           time.Time              `bson:"date"`
	MatFiles       []FileModel            `bson:"mat_files,omitempty"`
}
type VehicleRunModelResponse struct {
	Id             string                         `json:"id"`
	Date           time.Time                      `json:"date"`
	CarModel       string                         `json:"car_model"`
	SchemaVersions map[string]string              `json:"schema_versions"`
	Notes          *string                        `json:"notes"`
	McapFiles      []FileModelResponse            `json:"mcap_files"`
	MatFiles       []FileModelResponse            `json:"mat_files"`
	ContentFiles   map[string][]FileModelResponse `json:"content_files"`
	Location       *string                        `json:"location"`
	EventType      *string                        `json:"event_type"`
	DynamicFields  map[string]interface{}         `json:"dynamic_fields"`
}

func VehicleRunSerialize(ctx context.Context, s3Repo *s3.S3Repository, model VehicleRunModel) VehicleRunModelResponse {
	modelOut := VehicleRunModelResponse{
		Id:             model.Id.Hex(),
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
