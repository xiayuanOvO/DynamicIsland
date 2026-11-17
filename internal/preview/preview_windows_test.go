package preview

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// These tests exercise file/bitmap APIs only; they create no application windows.
func TestSystemImageThumbnail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "红色 图片.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 220, G: 50, B: 35, A: 255}), image.Point{}, draw.Src)
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatalf("write image: %v, %v", encodeErr, closeErr)
	}
	thumb, err := systemImage(path, true)
	if err != nil {
		t.Fatal(err)
	}
	bounds := thumb.Bounds()
	if bounds.Empty() || bounds.Dx() > Size || bounds.Dy() > Size {
		t.Fatalf("invalid native thumbnail size: %v", bounds)
	}
	pixel := color.NRGBAModel.Convert(thumb.At(bounds.Dx()/2, bounds.Dy()/2)).(color.NRGBA)
	if pixel.A == 0 || pixel.R < 150 || pixel.G > 120 || pixel.B > 120 {
		t.Fatalf("native bitmap lost its colors or alpha: %+v", pixel)
	}
}

func TestSystemFolderIcon(t *testing.T) {
	icon, err := systemImage(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Empty() || icon.Bounds().Dx() > 96 || icon.Bounds().Dy() > 96 {
		t.Fatalf("invalid native icon size: %v", icon.Bounds())
	}
	for y := range icon.Bounds().Dy() {
		for x := range icon.Bounds().Dx() {
			if _, _, _, alpha := icon.At(x, y).RGBA(); alpha > 0 {
				return
			}
		}
	}
	t.Fatal("native icon is fully transparent")
}
