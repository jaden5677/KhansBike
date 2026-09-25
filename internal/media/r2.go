package media

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// R2Store keeps blobs in a Cloudflare R2 bucket through R2's S3-compatible API.
type R2Store struct {
	client *minio.Client
	bucket string
}

// NewR2Store connects to the account's R2 endpoint. No request is made until
// the first Put or Open.
func NewR2Store(accountID, bucket, accessKeyID, secretAccessKey string) (*R2Store, error) {
	client, err := minio.New(accountID+".r2.cloudflarestorage.com", &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: true,
		Region: "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("media: r2 client: %w", err)
	}
	return &R2Store{client: client, bucket: bucket}, nil
}

// Put uploads a blob. Keys never change content (originals are content
// addressed, renditions are deterministic), so objects are cacheable forever.
func (s *R2Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	})
	if err != nil {
		return fmt.Errorf("media: r2 put %s: %w", key, err)
	}
	return nil
}

// Open fetches a blob. The object is stat'ed first so a missing key surfaces
// here as ErrBlobNotFound rather than on the first read.
func (s *R2Store) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("media: r2 get %s: %w", key, err)
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("media: r2 stat %s: %w", key, err)
	}
	return obj, nil
}
