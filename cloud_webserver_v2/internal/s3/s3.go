package s3

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Session struct {
	client *s3.Client
    bucket string
}

func NewS3Session(region string, bucket string) *S3Repository {
	// Load aws config (.aws/config)
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	// Create an aws s3 service client
	client := s3.NewFromConfig(cfg)

	session := &S3Session{
		client: client,
        bucket: bucket,
	}
	return &S3Repository{
		s3_session: session,
	}
}
