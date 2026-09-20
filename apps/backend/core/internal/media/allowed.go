package media

import "slices"

// SVG is deliberately excluded, because its ability to include scripts makes it a common XSS vector unless sanitized, which v1 doesn't do
// HEIC and HEIF are accepted because iPhones default to them, and rejecting them would block almost every phone photo uploaded alongside a review
var AllowedImageMimeTypes = []string{
	"image/jpeg",
	"image/png",
	"image/webp",
	"image/gif",
	"image/heic",
	"image/heif",
}

// Accepted on user-generated content surfaces, currently product reviews - kept narrow, with every entry having broad in-browser <video> support. iPhone .mov lands as video/quicktime, modern Android typically sends video/mp4
var AllowedVideoMimeTypes = []string{
	"video/mp4",
	"video/webm",
	"video/quicktime",
}

func IsAllowedImage(mime string) bool {
	return slices.Contains(AllowedImageMimeTypes, mime)
}

func IsAllowedVideo(mime string) bool {
	return slices.Contains(AllowedVideoMimeTypes, mime)
}

// The canonical file extension, leading dot included, for a known image OR video MIME type, used when building the S3 object key
func ExtensionForMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	}
	return ""
}
