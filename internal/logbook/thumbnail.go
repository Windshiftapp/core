package logbook

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const thumbnailMaxSize = 600
const previewMaxSize = 1200
const thumbnailJPEGQuality = 85

// pdftoppmTimeout caps how long pdftoppm may run on a single page render.
// Real-world first-page renders finish in well under a second; this is a
// generous ceiling for anti-hang only.
const pdftoppmTimeout = 60 * time.Second

// GenerateThumbnailAndPreview creates a JPEG thumbnail (600px) and a larger preview (1200px)
// for the given document file. Returns the two output paths on success, or ("", "", nil) if
// the mime type is unsupported. The source is decoded/rendered once and scaled twice.
func GenerateThumbnailAndPreview(docID, filePath, mimeType, outputDir string) (thumbPath, previewPath string, err error) {
	thumbPath = filepath.Join(outputDir, docID+".thumb.jpg")
	previewPath = filepath.Join(outputDir, docID+".preview.jpg")

	var img image.Image
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		img, err = decodeImage(filePath)
	case mimeType == "application/pdf":
		img, err = renderPDFFirstPage(filePath, thumbPath)
	default:
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}

	if err := scaleAndSaveJPEG(img, thumbPath, thumbnailMaxSize); err != nil {
		return "", "", err
	}
	if err := scaleAndSaveJPEG(img, previewPath, previewMaxSize); err != nil {
		return "", "", err
	}
	return thumbPath, previewPath, nil
}

// decodeImage decodes an image file from disk.
func decodeImage(inputPath string) (image.Image, error) {
	f, err := os.Open(inputPath) //nolint:gosec // G304 — inputPath from DB-stored path (UUID dirs + filepath.Base filename)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// renderPDFFirstPage uses pdftoppm to render the first page and returns it as an image.
// tmpAnchorPath is used only to derive a unique tmp-file prefix.
func renderPDFFirstPage(inputPath, tmpAnchorPath string) (image.Image, error) {
	// pdftoppm writes to <prefix>-<page>.jpg; with -singlefile it writes <prefix>.jpg
	tmpPrefix := tmpAnchorPath + ".tmp"
	tmpFile := tmpPrefix + ".jpg"
	defer func() { _ = os.Remove(tmpFile) }() //nolint:gosec // G703: tmpFile derived from UUID-based anchor path

	ctx, cancel := context.WithTimeout(context.Background(), pdftoppmTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftoppm", //nolint:gosec // G204: pdftoppm path from system, not user input
		"-jpeg", "-f", "1", "-l", "1", "-r", "300", "-singlefile",
		inputPath, tmpPrefix,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("pdftoppm timed out after %s", pdftoppmTimeout)
		}
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, string(out))
	}

	f, err := os.Open(tmpFile) //nolint:gosec // G304 — tmpFile derived from anchor path (UUID-based) + hardcoded suffix
	if err != nil {
		return nil, fmt.Errorf("open pdftoppm output: %w", err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode pdftoppm output: %w", err)
	}
	return img, nil
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
