package s3

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	mediaerrors "github.com/can3p/pcom/pkg/media/errors"
)

type s3Server struct {
	s3     *s3.Client
	bucket string
}

// Options is the bucket to store user media in.
type Options struct {
	Endpoint, Bucket, Region, Key, Secret string
	// PathStyle addresses objects as endpoint/bucket/key, which tommy needs.
	PathStyle bool
}

// New builds the client from o alone: it never reads AWS_* variables or
// ~/.aws.
func New(o Options) (*s3Server, error) {
	client := s3.New(s3.Options{
		Region:       o.Region,
		Credentials:  awscreds.NewStaticCredentialsProvider(o.Key, o.Secret, ""),
		BaseEndpoint: aws.String(o.Endpoint),
		UsePathStyle: o.PathStyle,
	})

	return &s3Server{
		s3:     client,
		bucket: o.Bucket,
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
			return nil, 0, "", mediaerrors.ErrNotFound
		}
		return nil, 0, "", err
	}

	return result.Body, *result.ContentLength, *result.ContentType, nil
}

func (s3s *s3Server) ObjectExists(ctx context.Context, fname string) (bool, error) {
	_, err := s3s.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s3s.bucket),
		Key:    aws.String(fname),
	})
	if err != nil {
		// HEAD has no body to name the error, so a missing key is NotFound
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
