package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Repository struct {
	s3_session *S3Session
}

func (s *S3Repository) WriteObject(writer *io.WriterTo, objectName string) {
	var buf bytes.Buffer

	_, err := (*writer).WriteTo(&buf)
	if err != nil {
		log.Printf("Failed to write buffer: %v", err)
	}

	reader := bytes.NewReader(buf.Bytes())
	_, err = s.s3_session.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.s3_session.bucket),
		Key:    aws.String(objectName),
		Body:   reader,
	})
	if err != nil {
		log.Printf("Couldn't upload file %v to %v:%v. Here's why: %v\n",
			objectName, s.s3_session.bucket, objectName, err)
	}
}

func (s *S3Repository) ListObjects() {
	result, err := s.s3_session.client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		fmt.Printf("Couldn't list buckets for your account. Here's why: %v\n", err)
		return
	}

	fmt.Printf("objects are %v \n", result)
}
