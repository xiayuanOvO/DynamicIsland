package preview

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeRasterPreservesAlphaAndOriginal(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 256))
	want := color.NRGBA{R: 210, G: 80, B: 40, A: 128}
	draw.Draw(img, img.Bounds(), image.NewUniform(want), image.Point{}, draw.Src)
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "透明 图片.png")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	thumb, err := decodeRaster(path)
	if err != nil {
		t.Fatal(err)
	}
	if thumb.Bounds().Dx() != 256 || thumb.Bounds().Dy() != 64 {
		t.Fatalf("wrong thumbnail aspect ratio or size: %v", thumb.Bounds())
	}
	got := color.NRGBAModel.Convert(thumb.At(128, 32)).(color.NRGBA)
	if got.A != want.A || abs(int(got.R)-int(want.R)) > 2 {
		t.Fatalf("transparent pixels changed: got %+v, want %+v", got, want)
	}
	original, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, data.Bytes()) {
		t.Fatalf("source image changed: %v", err)
	}
}

func TestDecodeCommonFormatsWithoutUpscaling(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	encoders := map[string]func(*bytes.Buffer) error{
		"png":  func(b *bytes.Buffer) error { return png.Encode(b, img) },
		"jpeg": func(b *bytes.Buffer) error { return jpeg.Encode(b, img, nil) },
		"gif":  func(b *bytes.Buffer) error { return gif.Encode(b, img, nil) },
	}
	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			var data bytes.Buffer
			if err := encode(&data); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "image."+name)
			if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			thumb, err := decodeRaster(path)
			if err != nil || thumb.Bounds().Dx() != 20 || thumb.Bounds().Dy() != 10 {
				t.Fatalf("small image was changed or unreadable: %v", err)
			}
		})
	}
}

func TestDecodeRejectsInvalidAndExcessiveImages(t *testing.T) {
	dir := t.TempDir()
	// A valid GIF header advertising 60000 x 60000 pixels must be rejected
	// before a decoder tries to allocate the full image.
	for name, data := range map[string][]byte{
		"broken.png": []byte("not an image"),
		"huge.gif":   {'G', 'I', 'F', '8', '9', 'a', 0x60, 0xea, 0x60, 0xea, 0, 0, 0},
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := decodeRaster(path); err == nil {
			t.Fatalf("invalid image accepted: %s", name)
		}
	}
	if _, err := decodeRaster(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("missing image accepted")
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
