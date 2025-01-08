package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/can3p/pcom/pkg/media"
)

type s3Server struct {
	s3     *s3.Client
	bucket string
}

var RequiredEnv = []string{"USER_MEDIA_ENDPOINT", "USER_MEDIA_BUCKET", "USER_MEDIA_KEY", "USER_MEDIA_REGION", "USER_MEDIA_SECRET"}

func NewS3Server() (*s3Server, error) {
	endpoint := os.Getenv("USER_MEDIA_ENDPOINT")
	bucket := os.Getenv("USER_MEDIA_BUCKET")
	key := os.Getenv("USER_MEDIA_KEY")
	region := os.Getenv("USER_MEDIA_REGION")
	secret := os.Getenv("USER_MEDIA_SECRET")

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: endpoint,
		}, nil
	})

	creds := awscreds.NewStaticCredentialsProvider(key, secret, "")

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(creds),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg)

	return &s3Server{
		s3:     s3Client,
		bucket: bucket,
	}, nil
}

func (s3s *s3Server) UploadFile(ctx context.Context, fname string, b []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s3s.bucket),
		Key:         aws.String(fname),
		Body:        bytes.NewReader(b),
		ACL:         types.ObjectCannedACLPrivate,
		ContentType: aws.String(contentType),
	}
	_, err := s3s.s3.PutObject(ctx, input)

	return err
}

func (s3s *s3Server) DownloadFile(ctx context.Context, fname string) (io.ReadCloser, int64, string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(fname),
	}

	result, err := s3s.s3.GetObject(ctx, input)

	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, 0, "", media.ErrNotFound
		}
		return nil, 0, "", err
	}

	return result.Body, *result.ContentLength, *result.ContentType, nil
}
