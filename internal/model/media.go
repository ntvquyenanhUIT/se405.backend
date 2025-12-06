package model

import "errors"

const (
	MaxAvatarSizeBytes = 5 * 1024 * 1024 // 5MB limit per media plan
	AvatarWidth        = 200
	AvatarHeight       = 200
	AvatarFolder       = "avatars"
	AvatarExt          = ".jpg"
	AvatarCacheControl = "public, max-age=31536000" // 1 year
)

// Supported image content types for upload validation
const (
	ContentTypeJPEG = "image/jpeg"
	ContentTypePNG  = "image/png"
	ContentTypeGIF  = "image/gif"
	ContentTypeWebP = "image/webp"
)

var allowedImageTypes = map[string]struct{}{
	ContentTypeJPEG: {},
	ContentTypePNG:  {},
	ContentTypeGIF:  {},
	ContentTypeWebP: {},
}

// Error codes for HTTP responses
const (
	CodeFileTooLarge     = "FILE_TOO_LARGE"
	CodeInvalidImageType = "INVALID_IMAGE_TYPE"
)

// Domain errors for media operations
var (
	ErrFileTooLarge     = errors.New("file too large")
	ErrInvalidImageType = errors.New("invalid image type")
)

// UploadResult represents the uploaded object location
// URL is the public-facing URL (using R2 public endpoint)
// Key is the object key inside the bucket (useful for future deletes)
type UploadResult struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// IsAllowedImageType reports if the provided content type is supported
func IsAllowedImageType(contentType string) bool {
	_, ok := allowedImageTypes[contentType]
	return ok
}
