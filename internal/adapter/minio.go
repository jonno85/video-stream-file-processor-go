package adapter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jonno85/video-stream-file-processor/internal/service/utils"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)


const (
	maxUploadAttempts = 5
	initialBackoff    = 1 * time.Millisecond
)

type S3ClientImpl struct {
	s3Client *minio.Client
}

func NewMinioClient() *S3ClientImpl {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	useSSL := false
	if strings.ToLower(os.Getenv("MINIO_USE_SSL")) == "true" {
		useSSL = true
	}
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		slog.Error("MINIO_ACCESS_KEY is not set")
		os.Exit(1)
	}
	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		slog.Error("MINIO_SECRET_KEY is not set")
		os.Exit(1)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		slog.Error("Failed to create MinIO client", "err", err)
		return nil
	}
	return &S3ClientImpl{
		s3Client: client,
	}
}

func (s *S3ClientImpl) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return s.s3Client.BucketExists(ctx, bucketName)
}

func (s *S3ClientImpl) MakeBucket(ctx context.Context, bucketName string, opts minio.MakeBucketOptions) error {
	return s.s3Client.MakeBucket(ctx, bucketName, opts)
}

func (s *S3ClientImpl) PutObjectWithIdempotency(ctx context.Context, bucketName string, objectName string, data io.Reader, hash string, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	objInfo, err := utils.Retry(maxUploadAttempts, initialBackoff, func() (any, error) {
		return s.s3Client.StatObject(context.Background(), bucketName, objectName, minio.StatObjectOptions{})
	})

	if err == nil {
		slog.Info("Chunk already exists", "chunkName", objectName)
		// MinIO/S3 prefixes user metadata with "X-Amz-Meta-"
		remoteHash := objInfo.(minio.ObjectInfo).UserMetadata["X-Amz-Meta-Hash"]
		if remoteHash == hash {
			slog.Info("Chunk already exists and hash matches", "chunkName", objectName, "remoteHash", remoteHash, "hash", hash)
			return minio.UploadInfo{}, nil
		} else {
			slog.Info("Chunk already exists but hash does not match", "chunkName", objectName, "remoteHash", remoteHash, "hash", hash)
			return minio.UploadInfo{}, errors.New("chunk already exists but hash does not match")
		}
	}
	
	slog.Debug("Uploading chunk", "chunkName", objectName)
	
	// upload data to s3
	info, err := utils.Retry(maxUploadAttempts, initialBackoff, func() (any, error) {
		return s.s3Client.PutObject(context.Background(), bucketName, objectName, data, size, opts)
	})
	return info.(minio.UploadInfo), err
}