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

type S3Repository struct {
	s3_session *S3Session
}

// Writes an object to the S3 bucket. You can think of an S3 object like a file.
// We store all our images, MATLAB, and MCAP files here.
func (s *S3Repository) WriteObject(ctx context.Context, writer *io.WriterTo, objectName string) {
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
		log.Printf("Couldn't upload file %v to %v:%v. Here's why: %v\n",
			objectName, s.s3_session.bucket, objectName, err)
	}
}

func (s *S3Repository) WriteObjectReader(ctx context.Context, reader io.Reader, objectName string) error {
	_, err := s.s3_session.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectName),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("Couldn't upload file %v to %v:%v. Here's why: %v\n",
			objectName, s.s3_session.bucket, objectName, err)
	}

	return nil
}

func (s *S3Repository) ListObjects(ctx context.Context) {
	result, err := s.s3_session.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		fmt.Printf("Couldn't list buckets for your account. Here's why: %v\n", err)
		return
	}

	fmt.Printf("objects are %v \n", result)
}

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

func (s *S3Repository) FileExists(ctx context.Context, objectName string) (bool, error) {
	_, err := s.s3_session.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectName),
	})

	if err != nil {
		return false, fmt.Errorf("error checking if file %v exists in bucket %v: %v", objectName, s.s3_session.bucket, err)
	}

	return true, nil
}

func (s *S3Repository) DeleteObject(ctx context.Context, objectName string) (bool, error) {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.s3_session.bucket), // s3 bucket name
		Key:    aws.String(objectName),          // file name
	}

	// Delete the object
	_, err := s.s3_session.client.DeleteObject(ctx, input)
	if err != nil {
		return false, fmt.Errorf("error deleting file named %v in bucket %v: %v", objectName, s.s3_session.bucket, err)
	}

	return true, nil
}
