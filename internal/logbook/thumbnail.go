package logbook

import (
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const thumbnailMaxSize = 600
const thumbnailJPEGQuality = 85

// GenerateThumbnail creates a JPEG thumbnail for the given document file.
// Returns the output path on success, or ("", nil) if the mime type is unsupported.
func GenerateThumbnail(docID, filePath, mimeType, outputDir string) (string, error) {
	outputPath := filepath.Join(outputDir, docID+".thumb.jpg")

	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return outputPath, generateImageThumbnail(filePath, outputPath)
	case mimeType == "application/pdf":
		return outputPath, generatePDFThumbnail(filePath, outputPath)
	default:
		return "", nil
	}
}

// generateImageThumbnail decodes an image file and scales it to a thumbnail.
func generateImageThumbnail(inputPath, outputPath string) error {
	f, err := os.Open(inputPath) //nolint:gosec // G304 — inputPath from DB-stored path (UUID dirs + filepath.Base filename)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}

	return scaleAndSaveJPEG(img, outputPath, thumbnailMaxSize)
}

// generatePDFThumbnail uses pdftoppm to render the first page, then scales it.
func generatePDFThumbnail(inputPath, outputPath string) error {
	// pdftoppm writes to <prefix>-<page>.jpg; with -singlefile it writes <prefix>.jpg
	tmpPrefix := outputPath + ".tmp"
	tmpFile := tmpPrefix + ".jpg"
	defer func() { _ = os.Remove(tmpFile) }()

	cmd := exec.Command("pdftoppm", //nolint:gosec // G204: pdftoppm path from system, not user input
		"-jpeg", "-f", "1", "-l", "1", "-r", "300", "-singlefile",
		inputPath, tmpPrefix,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm: %w: %s", err, string(out))
	}

	f, err := os.Open(tmpFile) //nolint:gosec // G304 — tmpFile derived from outputPath (UUID-based) + hardcoded suffix
	if err != nil {
		return fmt.Errorf("open pdftoppm output: %w", err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		return fmt.Errorf("decode pdftoppm output: %w", err)
	}

	return scaleAndSaveJPEG(img, outputPath, thumbnailMaxSize)
}

// scaleAndSaveJPEG scales an image to fit within maxSize x maxSize (preserving aspect ratio)
// and saves it as a JPEG file.
func scaleAndSaveJPEG(img image.Image, outputPath string, maxSize int) error {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Calculate scaled dimensions preserving aspect ratio
	newW, newH := w, h
	if w > maxSize || h > maxSize {
		if w >= h {
			newW = maxSize
			newH = h * maxSize / w
		} else {
			newH = maxSize
			newW = w * maxSize / h
		}
	}
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	out, err := os.Create(outputPath) //nolint:gosec // G304 — outputPath from UUID-based storage path
	if err != nil {
		return fmt.Errorf("create thumbnail file: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, dst, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
		return fmt.Errorf("encode jpeg: %w", err)
	}

	return nil
}
