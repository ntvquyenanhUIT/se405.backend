package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"
	"github.com/google/uuid"

	"iamstagram_22520060/internal/config"
	domain "iamstagram_22520060/internal/model"
)

// MediaService handles media uploads to Cloudflare R2.
type MediaService struct {
	s3Client  *s3.Client
	bucket    string
	publicURL string
}

// NewMediaService constructs an S3-compatible client for Cloudflare R2.
func NewMediaService(ctx context.Context, cfg *config.Config) (*MediaService, error) {
	if cfg.R2AccountID == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2BucketName == "" || cfg.R2PublicURL == "" {
		return nil, fmt.Errorf("missing Cloudflare R2 configuration")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for R2: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &MediaService{
		s3Client:  s3Client,
		bucket:    cfg.R2BucketName,
		publicURL: strings.TrimSuffix(cfg.R2PublicURL, "/"),
	}, nil
}

// UploadAvatar enforces size/type, normalizes to 200x200 JPEG, and uploads to R2.
func (s *MediaService) UploadAvatar(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*domain.UploadResult, error) {
	data, _, err := readAndValidateImage(file, header, domain.MaxAvatarSizeBytes)
	if err != nil {
		return nil, err
	}

	jpegBytes, err := resizeToJPEG(data, domain.AvatarWidth, domain.AvatarHeight, 85)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s/%s%s", domain.AvatarFolder, uuid.NewString(), domain.AvatarExt)

	if err := s.putObject(ctx, key, jpegBytes, domain.ContentTypeJPEG, domain.AvatarCacheControl); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s", s.publicURL, key)
	return &domain.UploadResult{URL: url, Key: key}, nil
}

// readAndValidateImage loads the upload into memory with size and type checks.
func readAndValidateImage(file multipart.File, header *multipart.FileHeader, maxSize int64) ([]byte, string, error) {
	if header.Size > maxSize {
		return nil, "", domain.ErrFileTooLarge
	}

	limitedReader := io.LimitReader(file, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read upload: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, "", domain.ErrFileTooLarge
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" && len(data) > 0 {
		contentType = http.DetectContentType(data[:min(len(data), 512)])
	}
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if !domain.IsAllowedImageType(contentType) {
		return nil, "", domain.ErrInvalidImageType
	}

	return data, contentType, nil
}

// resizeToJPEG centers/crops to target size and encodes as JPEG.
func resizeToJPEG(data []byte, width, height, quality int) ([]byte, error) {
	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	resized := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, fmt.Errorf("failed to encode jpeg: %w", err)
	}

	return buf.Bytes(), nil
}

// putObject uploads bytes to R2 with metadata.
func (s *MediaService) putObject(ctx context.Context, key string, body []byte, contentType, cacheControl string) error {
	_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(body),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(cacheControl),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to r2: %w", err)
	}
	return nil
}

// DeleteObject removes an object by key. Callers should ensure the key is not the shared default.
func (s *MediaService) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from r2: %w", err)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
