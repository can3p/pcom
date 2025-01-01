package s3

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/can3p/pcom/pkg/media"
)

type s3Server struct {
	s3     *s3.S3
	bucket string
}

var RequiredEnv = []string{"USER_MEDIA_ENDPOINT", "USER_MEDIA_BUCKET", "USER_MEDIA_KEY", "USER_MEDIA_REGION", "USER_MEDIA_SECRET"}

func NewS3Server() (*s3Server, error) {
	endpoint := os.Getenv("USER_MEDIA_ENDPOINT")
	bucket := os.Getenv("USER_MEDIA_BUCKET")
	key := os.Getenv("USER_MEDIA_KEY")
	region := os.Getenv("USER_MEDIA_REGION")
	secret := os.Getenv("USER_MEDIA_SECRET")

	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(region),
		S3ForcePathStyle: aws.Bool(false), // // Configures to use subdomain/virtual calling format. Depending on your version, alternatively use o.UsePathStyle = false
	}

	newSession, err := session.NewSession(s3Config)

	if err != nil {
		return nil, err
	}

	s3Client := s3.New(newSession)

	return &s3Server{
		s3:     s3Client,
		bucket: bucket,
	}, nil
}

func (s3s *s3Server) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	object := s3.PutObjectInput{
		Bucket:      aws.String(s3s.bucket),
		Key:         aws.String(fname),
		Body:        bytes.NewReader(b),
		ACL:         aws.String("private"),
		ContentType: aws.String(contentType),
	}
	_, err := s3s.s3.PutObject(&object)

	return err
}

func (s3s *s3Server) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(fname),
	}

	result, err := s3s.s3.GetObject(input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == s3.ErrCodeNoSuchKey {
				return nil, 0, "", media.ErrNotFound
			}
		}

		return nil, 0, "", err
	}

	return result.Body, *result.ContentLength, *result.ContentType, nil
}
