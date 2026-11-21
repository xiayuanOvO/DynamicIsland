package island

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"dynamicisland/internal/nowplaying"
	"github.com/egoist/mygo/ui"
)

const (
	maxArtworkBytes     = 8 << 20
	maxArtworkCacheSize = 64 << 20
)

func resolveArtwork(track, previous nowplaying.Track, currentHash [32]byte, cacheDir string) (cover ui.ImageSource, color ui.Color, hash [32]byte, keepCover bool) {
	if track.Title == "" {
		return
	}
	key, previousKey := track.ArtworkKey(), previous.ArtworkKey()
	keepCover = previous.Title != "" && (key == previousKey || track.SameSong(previous))
	if len(track.Artwork) > 0 && len(track.Artwork) <= maxArtworkBytes {
		hash = sha256.Sum256(track.Artwork)
		if key == previousKey && hash == currentHash {
			return
		}
		cover, color = decodeArtwork(track.Artwork)
		if cover != nil {
			saveArtwork(cacheDir, key, track.Artwork)
			return
		}
	}
	if cacheDir == "" || (keepCover && currentHash != [32]byte{}) {
		return
	}
	path := artworkCachePath(cacheDir, key)
	file, err := os.Open(path)
	if err != nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxArtworkBytes+1))
	file.Close()
	if err != nil {
		return
	}
	cover, color = decodeArtwork(data)
	if cover != nil {
		hash = sha256.Sum256(data)
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return
}

func artworkCachePath(dir string, key [4]string) string {
	encoded := fmt.Sprintf("%q %q %q %q", key[0], key[1], key[2], key[3])
	return filepath.Join(dir, fmt.Sprintf("%x.cover", sha256.Sum256([]byte(encoded))))
}

func saveArtwork(dir string, key [4]string, data []byte) {
	if dir == "" || os.MkdirAll(dir, 0700) != nil {
		return
	}
	file, err := os.CreateTemp(dir, ".artwork-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if closeErr := file.Close(); writeErr != nil || closeErr != nil {
		return
	}
	if os.Rename(file.Name(), artworkCachePath(dir, key)) != nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var total int64
	var covers []fs.FileInfo
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".cover" {
			continue
		}
		if info, err := entry.Info(); err == nil {
			covers = append(covers, info)
			total += info.Size()
		}
	}
	if total <= maxArtworkCacheSize {
		return
	}
	slices.SortFunc(covers, func(a, b fs.FileInfo) int { return a.ModTime().Compare(b.ModTime()) })
	for _, cover := range covers {
		if os.Remove(filepath.Join(dir, cover.Name())) == nil {
			total -= cover.Size()
			if total <= maxArtworkCacheSize {
				break
			}
		}
	}
}
