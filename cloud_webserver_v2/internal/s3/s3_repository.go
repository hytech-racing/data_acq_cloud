package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	transfermanagertypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrObjectNotFound is returned when an object does not exist in the bucket.
var ErrObjectNotFound = errors.New("s3 object not found")

// MultipartPart identifies a single part of a multipart upload. The ETag is the value
// S3 returned to the client when the part was uploaded directly to the bucket.
type MultipartPart struct {
	PartNumber int32
	ETag       string
}

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
		opts.Expires = time.Duration(24 * int64(time.Hour))
	})
	if err != nil {
		log.Fatalf("Couldn't get a presigned request to get %v:%v: %v", bucket, objectPath, err)
	}

	return request.URL
}

// PresignPutObject returns a URL the client can PUT a single object to. The request is
// signed server-side, so the client never receives AWS credentials. ContentType is part
// of the signature, which stops a client from uploading a different kind of file than
// the one the server validated.
func (s *S3Repository) PresignPutObject(ctx context.Context, objectKey string, contentType string, expires time.Duration) (string, error) {
	request, err := s.s3_session.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.s3_session.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("couldn't presign put for %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return request.URL, nil
}

// CreateMultipartUpload starts a multipart upload and returns its upload ID. Multipart
// uploads are what lift the 5GB limit of a single presigned PUT: the client uploads
// independent parts and can retry only the parts that fail.
func (s *S3Repository) CreateMultipartUpload(ctx context.Context, objectKey string, contentType string) (string, error) {
	output, err := s.s3_session.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.s3_session.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("couldn't create multipart upload for %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return aws.ToString(output.UploadId), nil
}

// PresignUploadPart returns a URL for one part of an in-progress multipart upload.
func (s *S3Repository) PresignUploadPart(ctx context.Context, objectKey string, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	request, err := s.s3_session.presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(s.s3_session.bucket),
		Key:        aws.String(objectKey),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("couldn't presign part %d of %v:%v: %w", partNumber, s.s3_session.bucket, objectKey, err)
	}

	return request.URL, nil
}

// CompleteMultipartUpload tells S3 to assemble the uploaded parts into one object.
// Parts must be ordered by part number.
func (s *S3Repository) CompleteMultipartUpload(ctx context.Context, objectKey string, uploadID string, parts []MultipartPart) error {
	completedParts := make([]types.CompletedPart, len(parts))
	for i, part := range parts {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(part.PartNumber),
		}
	}

	_, err := s.s3_session.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.s3_session.bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("couldn't complete multipart upload for %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return nil
}

// AbortMultipartUpload discards the parts that were uploaded for an unfinished
// multipart upload so they stop costing storage.
func (s *S3Repository) AbortMultipartUpload(ctx context.Context, objectKey string, uploadID string) error {
	_, err := s.s3_session.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.s3_session.bucket),
		Key:      aws.String(objectKey),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return fmt.Errorf("couldn't abort multipart upload for %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return nil
}

// HeadObject returns the size in bytes of an object, or ErrObjectNotFound when it does
// not exist. It is used to confirm that a direct upload actually landed before the file
// is handed to the file processor.
func (s *S3Repository) HeadObject(ctx context.Context, objectKey string) (int64, error) {
	output, err := s.s3_session.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return 0, ErrObjectNotFound
		}
		return 0, fmt.Errorf("couldn't get metadata for %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return aws.ToInt64(output.ContentLength), nil
}

// GetObjectReader opens an object for streaming. The caller owns the returned reader
// and must close it.
func (s *S3Repository) GetObjectReader(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	output, err := s.s3_session.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("couldn't get %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return output.Body, nil
}

// DownloadObject stages an object into writer with the transfer manager's concurrent
// downloader and returns the number of bytes written.
func (s *S3Repository) DownloadObject(ctx context.Context, objectKey string, writer io.WriterAt) (int64, error) {
	out, err := s.s3_session.transferClient.DownloadObject(ctx, &transfermanager.DownloadObjectInput{
		Bucket:   aws.String(s.s3_session.bucket),
		Key:      aws.String(objectKey),
		WriterAt: writer,
	}, func(o *transfermanager.Options) {
		// Ranged parts make every object concurrent, not only multipart uploads.
		o.GetObjectType = transfermanagertypes.GetObjectRanges
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return 0, ErrObjectNotFound
		}
		return 0, fmt.Errorf("couldn't download %v:%v: %w", s.s3_session.bucket, objectKey, err)
	}

	return aws.ToInt64(out.ContentLength), nil
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
