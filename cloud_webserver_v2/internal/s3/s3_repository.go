package s3

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Repository allows for the server to interface with S3
type S3Repository struct {
	s3_session *s3Session
}

// Writes an object to the S3 bucket from a writer. You can think of an S3 object like a file.
// We store all our images, MATLAB, and MCAP files here.
func (s *S3Repository) PutObjectWithWriterTo(ctx context.Context, writer *io.WriterTo, objectName string) error {
	if writer == nil || *writer == nil {
		return fmt.Errorf("couldn't upload file %v to %v: writer is nil", objectName, s.s3_session.bucket)
	}

	// Writers that can also be read from (e.g. *bytes.Buffer, *os.File) are handed
	// over as-is so the transfer manager can size the object up front.
	if reader, ok := (*writer).(io.Reader); ok {
		return s.PutObject(ctx, reader, objectName)
	}

	// Otherwise pump the writer through a pipe so the object is streamed to S3
	// instead of being buffered in memory in its entirety.
	pipeReader, pipeWriter := io.Pipe()
	// Closing the reader on the way out unblocks the goroutine below if the
	// upload stops reading early because it failed.
	defer pipeReader.Close()
	go func() {
		_, err := (*writer).WriteTo(pipeWriter)
		// A nil error surfaces to the reader as io.EOF.
		pipeWriter.CloseWithError(err)
	}()

	return s.PutObject(ctx, pipeReader, objectName)
}

// Writes an object to the S3 bucket from a reader. Uploads are handled by the S3
// transfer manager, which transparently switches to a multipart upload so that
// objects larger than the 5GB single PutObject limit are supported.
func (s *S3Repository) PutObject(ctx context.Context, reader io.Reader, objectName string) error {
	_, err := s.s3_session.transferClient.UploadObject(ctx, &transfermanager.UploadObjectInput{
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
