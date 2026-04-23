package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"

)

type FoxgloveConfigModel struct {
	Id       primitive.ObjectID `bson:"_id,omitempty"`
	File	 FileModel          `bson:"file,omitempty"`
	// TODO: Add Information we want to track, empty for now as we haven't decided
}

type FoxgloveConfigModelResponse struct {
	Id       string `json:"_id"`
	File	 FileModelResponse          `json:"file"`
	// TODO: Add Information we want to track, empty for now as we haven't decided
}

func FoxgloveConfigSerialize(ctx context.Context, s3Repo *s3.S3Repository, model FoxgloveConfigModel) FoxgloveConfigModelResponse {
	modelOut := FoxgloveConfigModelResponse{
		Id:             model.Id.Hex(),
	}

	fileResponse := getFileModelResponse(ctx, s3Repo, []FileModel{model.File})
	// TODO: handle len(fileResponse) != 1 case
	modelOut.File = fileResponse[0]

	return modelOut
}