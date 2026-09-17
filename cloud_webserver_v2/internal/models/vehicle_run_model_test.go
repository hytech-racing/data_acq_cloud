package models

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hytech-racing/cloud-webserver-v2/internal/s3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func newTestVehicleRun(mcapFiles []FileModel, matFiles []FileModel) VehicleRunModel {
	return VehicleRunModel{
		Id:        primitive.NewObjectID(),
		Date:      time.Now(),
		CarModel:  "HT09",
		McapFiles: mcapFiles,
		MatFiles:  matFiles,
	}
}

func testS3Repository() *s3.S3Repository {
	return s3.NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", "http://localhost:9000", false)
}

func TestVehicleRunSerializeMarksRunWithoutHdf5FileAsMcapOnly(t *testing.T) {
	run := newTestVehicleRun([]FileModel{
		{AwsBucket: "test-bucket", FilePath: "run-id/run.mcap", FileName: "run.mcap", FileHash: "hash"},
	}, nil)

	response := VehicleRunSerialize(context.Background(), testS3Repository(), run)

	if !response.McapOnly {
		t.Fatalf("expected a run without an hdf5 file to be marked as mcap only")
	}
	if len(response.McapFiles) != 1 {
		t.Fatalf("expected the mcap file to still be returned, got %d files", len(response.McapFiles))
	}
	if response.MatFiles != nil {
		t.Fatalf("expected no hdf5 files to be returned, got %d files", len(response.MatFiles))
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("could not marshal the vehicle run response: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("could not unmarshal the vehicle run response: %v", err)
	}

	if mcapOnly, ok := decoded["mcap_only"].(bool); !ok || !mcapOnly {
		t.Fatalf("expected the \"mcap_only\" field to be true in the response, got %v", decoded["mcap_only"])
	}
}

func TestVehicleRunSerializeDoesNotMarkConvertedRunAsMcapOnly(t *testing.T) {
	run := newTestVehicleRun(
		[]FileModel{{AwsBucket: "test-bucket", FilePath: "run-id/run.mcap", FileName: "run.mcap"}},
		[]FileModel{{AwsBucket: "test-bucket", FilePath: "run-id/run.h5", FileName: "run.h5"}},
	)

	response := VehicleRunSerialize(context.Background(), testS3Repository(), run)

	if response.McapOnly {
		t.Fatalf("expected a run with an hdf5 file to not be marked as mcap only")
	}
	if len(response.MatFiles) != 1 {
		t.Fatalf("expected the hdf5 file to be returned, got %d files", len(response.MatFiles))
	}
}
