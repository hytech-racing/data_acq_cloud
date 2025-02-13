package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Repository allows for the server to interface with S3
type S3Repository struct {
	s3_session *s3Session
}

// Writes an object to the S3 bucket from a writer. You can think of an S3 object like a file.
// We store all our images, MATLAB, and MCAP files here.
func (s *S3Repository) WriteObjectWriterTo(ctx context.Context, writer *io.WriterTo, objectName string) error {
	var buf bytes.Buffer

	_, err := (*writer).WriteTo(&buf)
	if err != nil {
		log.Printf("Failed to write buffer: %v", err)
	}

	reader := bytes.NewReader(buf.Bytes())
	_, err = s.s3_session.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectName),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("couldn't upload file %v to %v:%v. Here's why: %v",
			objectName, s.s3_session.bucket, objectName, err)
	}

	return nil
}

// Writes an object to the S3 bucket from a reader.
func (s *S3Repository) WriteObjectReader(ctx context.Context, reader io.Reader, objectName string) error {
	_, err := s.s3_session.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectName),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("couldn't upload file %v to %v:%v. Here's why: %v",
			objectName, s.s3_session.bucket, objectName, err)
	}

	return nil
}

// Returns a list of objects in S3
func (s *S3Repository) ListObjects(ctx context.Context) {
	result, err := s.s3_session.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		fmt.Printf("Couldn't list buckets for your account. Here's why: %v\n", err)
		return
	}

	fmt.Printf("objects are %v \n", result)
}

// GetSignedUrl locates a valid object in S3 and responds with a presigned URL valid for 10 minutes
func (s *S3Repository) GetSignedUrl(ctx context.Context, bucket string, objectPath string) string {
	request, err := s.s3_session.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectPath),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(10 * int64(time.Minute))
	})
	if err != nil {
		log.Fatalf("Couldn't get a presigned request to get %v:%v: %v", bucket, objectPath, err)
	}

	return request.URL
}

// DeleteObject deletes an object from S3 located at the bucket and object path
func (s *S3Repository) DeleteObject(ctx context.Context, bucket string, objectPath string) error {
	params := s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &objectPath,
	}
	_, err := s.s3_session.client.DeleteObject(ctx, &params)
	if err != nil {
		return err
	}

	return nil
}

func (s *S3Repository) Bucket() string {
	return s.s3_session.bucket
}
