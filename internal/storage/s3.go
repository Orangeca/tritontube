package storage

import (
	"context"
	"io"
)

// S3Uploader abstracts S3 uploads for DASH segment backups.
type S3Uploader interface {
	UploadSegment(ctx context.Context, bucket, key string, body io.Reader) error
}

// NoopS3Uploader can be used in local development to disable S3 interactions.
type NoopS3Uploader struct{}

// UploadSegment implements the S3Uploader interface.
func (NoopS3Uploader) UploadSegment(ctx context.Context, bucket, key string, body io.Reader) error {
	_ = ctx
	_ = bucket
	_ = key
	if body != nil {
		_, _ = io.Copy(io.Discard, body)
	}
	return nil
}

// BufferedS3Uploader allows plugging arbitrary upload functions without pulling the AWS SDK.
type BufferedS3Uploader struct {
	UploadFunc func(ctx context.Context, bucket, key string, body io.Reader) error
}

// UploadSegment delegates to UploadFunc.
func (u BufferedS3Uploader) UploadSegment(ctx context.Context, bucket, key string, body io.Reader) error {
	if u.UploadFunc == nil {
		return nil
	}
	return u.UploadFunc(ctx, bucket, key, body)
}
