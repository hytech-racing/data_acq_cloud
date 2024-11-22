package s3

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func TestS3_URL(t *testing.T) {
	t.Log("Starting tests...")

	err := godotenv.Load(".env")
	if err != nil {
		t.Errorf("Error loading .env file %s", err)
	}

	// Setup aws s3 connection
	aws_region := os.Getenv("AWS_REGION")
	if aws_region == "" {
		t.Errorf("could not get aws region environment variable")
	}

	aws_bucket := os.Getenv("AWS_S3_RUN_BUCKET")
	if aws_bucket == "" {
		t.Errorf("could not get aws run bucket environment variable")
	}

	awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
	if awsAccessKey == "" {
		t.Errorf("could not get aws access key environment variable")
	}

	awsSecretKey := os.Getenv("AWS_SECRET_KEY")
	if awsSecretKey == "" {
		t.Errorf("could not get aws secret key environment variable")
	}

	s3_respository := NewS3Session(awsAccessKey, awsSecretKey, aws_region, aws_bucket)

	var ctx = context.Background()
	file, err := os.Open("test.txt")
	if err != nil {
		t.Errorf("Error opening file %s", err)
	}

	// Write the object to S3
	obj_name := "s3_test_obj"

	err = s3_respository.WriteObjectReader(ctx, file, obj_name)
	if err != nil {
		t.Errorf("Failed to write object to S3: %v", err)
		return
	}

	t.Log("Successfully wrote object to S3")

	exists, err := s3_respository.FileExists(ctx, obj_name)
	if err != nil {
		t.Errorf("File doesn't exist: %v", err)
	}

	// Retrieve and Check Signed URL
	signed_url := s3_respository.GetSignedUrl(ctx, aws_bucket, obj_name)
	t.Log("Signed URL: " + signed_url)

	resp, err := http.Get(signed_url)
	if err != nil {
		t.Errorf("Signed url is null: %v", err)
	}

	goodResp := resp.StatusCode == http.StatusOK

	deleted, err := s3_respository.DeleteObject(ctx, obj_name)
	if err != nil {
		t.Errorf("Unable to delete file: %v", err)
		return
	}

	want := true
	got := exists && goodResp && deleted

	if got != want {
		t.Errorf("Test fail! want: '%t', got: '%t'", want, got)
	}

	t.Log("Successfully deleted object from S3")
}
