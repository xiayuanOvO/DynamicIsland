package lyrics

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Cache struct {
	dir string
}

func NewCache(dataDir string) Cache {
	return Cache{dir: filepath.Join(dataDir, "lyrics-cache")}
}

func (c Cache) path(song Song) string {
	key := fmt.Sprintf("%q %q %q %g", song.Title, song.Artist, song.Album, song.Duration)
	return filepath.Join(c.dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(key))))
}

func (c Cache) Save(song Song, lines Lines) error {
	data, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(c.dir, ".lyrics-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	if err := temp.Close(); writeErr != nil || err != nil {
		return errors.Join(writeErr, err)
	}
	return os.Rename(temp.Name(), c.path(song))
}

func (c Cache) Size() (int64, error) {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var size int64
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		size += info.Size()
	}
	return size, nil
}

func (c Cache) Clear() error {
	return os.RemoveAll(c.dir)
}
