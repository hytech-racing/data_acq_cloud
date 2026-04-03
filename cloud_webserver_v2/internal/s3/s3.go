package s3

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Session establishes a new connection with S3
type s3Session struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

func NewS3Session(accessKey string, secretKey string, region string, bucket string, endpoint string) *S3Repository {
	staticCreds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(staticCreds))

	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	isDev := os.Getenv("ENV") == "DEVELOPMENT"

	// Create an aws s3 service client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = isDev
	})

	// In dev, presigned URLs need to use localhost so they're accessible from the browser
	presignClient := s3.NewPresignClient(client)
	if isDev {
		publicClient := s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String("http://localhost:3900")
			o.UsePathStyle = true
		})
		presignClient = s3.NewPresignClient(publicClient)
	}

	session := &s3Session{
		client:        client,
		bucket:        bucket,
		presignClient: presignClient,
	}
	return &S3Repository{
		s3_session: session,
	}
}
