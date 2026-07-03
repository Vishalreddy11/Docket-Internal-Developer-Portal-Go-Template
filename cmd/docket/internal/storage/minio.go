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

var tracer = otel.Tracer("docket/storage/minio")

type minioStorage struct {
	client *minio.Client
	bucket string
}

func newMinIO(ctx context.Context, cfg config.MinIOConfig, log *slog.Logger) (*minioStorage, error) {
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
		log.Info("minio bucket created", "bucket", cfg.Bucket)
	}
	log.Info("minio storage connected", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket)
	return &minioStorage{client: c, bucket: cfg.Bucket}, nil
}

func (m *minioStorage) Put(ctx context.Context, id string, r io.Reader, size int64, ct string) error {
	ctx, span := tracer.Start(ctx, "minio.PutObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", m.bucket),
		attribute.String("minio.object", id),
		attribute.Int64("minio.size", size),
		attribute.String("minio.content_type", ct),
	)
	_, err := m.client.PutObject(ctx, m.bucket, id, r, size, minio.PutObjectOptions{ContentType: ct})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (m *minioStorage) Get(ctx context.Context, id string) (io.ReadCloser, int64, string, error) {
	ctx, span := tracer.Start(ctx, "minio.GetObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", m.bucket),
		attribute.String("minio.object", id),
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
	span.SetAttributes(attribute.Int64("minio.size", stat.Size))
	return obj, stat.Size, stat.ContentType, nil
}

func (m *minioStorage) Delete(ctx context.Context, id string) error {
	ctx, span := tracer.Start(ctx, "minio.RemoveObject")
	defer span.End()
	span.SetAttributes(
		attribute.String("minio.bucket", m.bucket),
		attribute.String("minio.object", id),
	)
	err := m.client.RemoveObject(ctx, m.bucket, id, minio.RemoveObjectOptions{})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (m *minioStorage) Mode() string                  { return "live" }
func (m *minioStorage) Close(_ context.Context) error { return nil }
