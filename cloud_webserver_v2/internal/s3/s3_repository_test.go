package s3

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPresignPutObjectSignsContentType(t *testing.T) {
	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", "http://localhost:9000", true)

	signed, err := repository.PresignPutObject(context.Background(), "direct-uploads/user/id/run.mcap", "application/octet-stream", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutObject() returned unexpected error: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("could not parse signed URL: %v", err)
	}

	if parsed.Path != "/test-bucket/direct-uploads/user/id/run.mcap" {
		t.Errorf("signed URL path = %q", parsed.Path)
	}
	if algorithm := parsed.Query().Get("X-Amz-Algorithm"); algorithm != "AWS4-HMAC-SHA256" {
		t.Errorf("X-Amz-Algorithm = %q", algorithm)
	}
	if expires := parsed.Query().Get("X-Amz-Expires"); expires != "300" {
		t.Errorf("X-Amz-Expires = %q, want 300", expires)
	}

	signedHeaders, err := url.QueryUnescape(parsed.Query().Get("X-Amz-SignedHeaders"))
	if err != nil {
		t.Fatalf("could not decode signed headers: %v", err)
	}
	for _, header := range strings.Split(signedHeaders, ";") {
		t.Logf("signed header: %s", header)
	}
}

func TestPresignUploadPartIncludesPartNumber(t *testing.T) {
	repository := NewS3Session("access-key", "secret-key", "us-east-1", "test-bucket", "http://localhost:9000", true)

	signed, err := repository.PresignUploadPart(context.Background(), "direct-uploads/user/id/run.mcap", "upload-id", 7, 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignUploadPart() returned unexpected error: %v", err)
	}

	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("could not parse signed URL: %v", err)
	}
	if partNumber := parsed.Query().Get("partNumber"); partNumber != "7" {
		t.Errorf("partNumber = %q, want 7", partNumber)
	}
	if uploadID := parsed.Query().Get("uploadId"); uploadID != "upload-id" {
		t.Errorf("uploadId = %q, want upload-id", uploadID)
	}
}
