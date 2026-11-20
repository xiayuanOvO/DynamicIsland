package preview

import (
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const Size = 256

func IsImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".jfif", ".gif", ".webp", ".bmp", ".tif", ".tiff",
		".svg", ".heic", ".heif", ".avif", ".ico":
		return true
	}
	return false
}

// Load asks the system for a thumbnail or file icon, falling back to a
// bounded raster decoder for images. It must run outside the UI thread.
func Load(path string, thumbnail bool) (image.Image, error) {
	if img, err := systemImage(path, thumbnail); err == nil {
		return img, nil
	}
	if !thumbnail {
		return nil, errors.New("system file icon unavailable")
	}
	return decodeRaster(path)
}

func decodeRaster(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 32<<20 {
		return nil, errors.New("image is too large for the fallback decoder")
	}
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return nil, errors.New("invalid or excessive image dimensions")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return thumbnail(img, config.Width, config.Height), nil
}

// Thumbnail scales an image so its longest side fits within Size pixels.
func Thumbnail(img image.Image) image.Image {
	bounds := img.Bounds()
	return thumbnail(img, bounds.Dx(), bounds.Dy())
}

func thumbnail(img image.Image, w, h int) image.Image {
	if longest := max(w, h); longest > Size {
		w, h = max(1, w*Size/longest), max(1, h*Size/longest)
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), img, img.Bounds(), draw.Src, nil)
	return thumb
}
