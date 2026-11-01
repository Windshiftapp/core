package services

import (
	"bytes"
	"image"
	_ "image/gif" // Register GIF decoder for thumbnails.
	"image/jpeg"
	_ "image/png" // Register PNG decoder for thumbnails.
	"io"

	"windshift/internal/utils"

	"golang.org/x/image/draw"
)

// GenerateAttachmentThumbnailJPEG scales an uploaded image down to a 200px
// JPEG. It works from bytes so the same path serves local and object storage.
func GenerateAttachmentThumbnailJPEG(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	// Reject pixel bombs before the full decode allocates.
	if err := utils.EnsureImageDimensionsBounded(reader, maxAttachmentThumbnailSourcePixels); err != nil {
		return nil, err
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()
	const maxSize = 200
	var newWidth, newHeight int
	if origWidth > origHeight {
		newWidth = maxSize
		newHeight = (origHeight * maxSize) / origWidth
	} else {
		newHeight = maxSize
		newWidth = (origWidth * maxSize) / origHeight
	}
	if newWidth < 1 || newHeight < 1 {
		newWidth, newHeight = 1, 1
	}

	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, bounds, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, thumbnail, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
