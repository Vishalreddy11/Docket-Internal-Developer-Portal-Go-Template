package storage

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/example/docket/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// The MinIO Go SDK is a generic S3 client library — it works against any
// S3-compatible endpoint (SeaweedFS in this template, AWS S3 / Ceph RGW /
// artifactory-S3 in production forks). The "minio" in the import path is
// the library's name, not a MinIO server dependency.
var tracer = otel.Tracer("docket/storage/s3")

type s3Storage struct {
	client *minio.Client
	bucket string
}

func newS3(ctx context.Context, cfg config.S3Config, log *slog.Logger) (*s3Storage, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	exists, err := c.BucketExists(probeCtx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := c.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
		log.Info("s3 bucket created", "bucket", cfg.Bucket)
	}
	log.Info("s3 storage connected", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket)
	return &s3Storage{client: c, bucket: cfg.Bucket}, nil
}

func (m *s3Storage) Put(ctx context.Context, id string, r io.Reader, size int64, ct string) error {
	ctx, span := tracer.Start(ctx, "s3.PutObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.object", id),
		attribute.Int64("s3.size", size),
		attribute.String("s3.content_type", ct),
	)
	_, err := m.client.PutObject(ctx, m.bucket, id, r, size, minio.PutObjectOptions{ContentType: ct})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (m *s3Storage) Get(ctx context.Context, id string) (io.ReadCloser, int64, string, error) {
	ctx, span := tracer.Start(ctx, "s3.GetObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.object", id),
	)
	obj, err := m.client.GetObject(ctx, m.bucket, id, minio.GetObjectOptions{})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, "", err
	}
	stat, err := obj.Stat()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		obj.Close()
		return nil, 0, "", err
	}
	span.SetAttributes(attribute.Int64("s3.size", stat.Size))
	return obj, stat.Size, stat.ContentType, nil
}

func (m *s3Storage) Delete(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "s3.RemoveObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("s3.bucket", m.bucket),
		attribute.String("s3.object", id),
	)
	err := m.client.RemoveObject(ctx, m.bucket, id, minio.RemoveObjectOptions{})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (m *s3Storage) Mode() string                  { return "live" }
func (m *s3Storage) Close(_ context.Context) error { return nil }
