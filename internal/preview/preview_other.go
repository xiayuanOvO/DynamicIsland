//go:build !windows

package preview

import (
	"bytes"
	"errors"
	"image"

	"github.com/egoist/mygo"
)

func systemImage(path string, thumbnail bool) (image.Image, error) {
	if thumbnail {
		return nil, errors.New("native thumbnails unavailable")
	}
	data, err := mygo.Shell.FileIcon(path, 96)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
