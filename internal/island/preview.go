package island

import (
	"bytes"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dynamicisland/internal/preview"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo/ui"
)

type filePreview struct {
	source    ui.ImageSource
	thumbnail bool
	pending   bool
	used      uint64
}

type previewJob struct {
	key   string
	entry *filePreview
	load  func() ui.ImageSource
}

func (a *island) startPreviews() {
	a.previews = make(map[string]*filePreview)
	a.previewJobs = make(chan previewJob, 16)
	for range 2 {
		go func() {
			for job := range a.previewJobs {
				source := job.load()
				a.win.Update(func() {
					// A removed or replaced card must not acquire an old result.
					if a.previews[job.key] == job.entry {
						job.entry.source, job.entry.pending = source, false
					}
				})
			}
		}()
	}
}

// Only visible cards request work. The bounded queue and cache keep a
// large shelf from decoding all its images or retaining their full sizes.
func (a *island) filePreview(item shelf.Item) *filePreview {
	thumbnail := !item.Directory && preview.IsImage(item.Path)
	return a.queuePreview(shelf.Key(item.Path), thumbnail, func() ui.ImageSource {
		if img, err := preview.Load(item.Path, thumbnail); err == nil {
			return ui.NewBitmap(img)
		}
		if thumbnail && strings.EqualFold(filepath.Ext(item.Path), ".svg") {
			return svgPreview(item.Path)
		}
		return nil
	})
}

func (a *island) clipboardPreview(id int64) *filePreview {
	store := a.clipboard.store
	return a.queuePreview("clipboard:"+strconv.FormatInt(id, 10), true, func() ui.ImageSource {
		var bitmap []byte
		if err := store.QueryRow("SELECT image FROM entries WHERE id=?", id).Scan(&bitmap); err != nil {
			return nil
		}
		if img, err := png.Decode(bytes.NewReader(bitmap)); err == nil {
			return ui.NewBitmap(preview.Thumbnail(img))
		}
		return nil
	})
}

func (a *island) queuePreview(key string, thumbnail bool, load func() ui.ImageSource) *filePreview {
	a.previewClock++
	if entry := a.previews[key]; entry != nil {
		entry.used = a.previewClock
		return entry
	}
	if a.previewJobs == nil {
		return nil
	}
	if len(a.previews) >= 96 {
		oldestKey := ""
		oldestUse := ^uint64(0)
		for key, entry := range a.previews {
			if !entry.pending && entry.used < oldestUse {
				oldestKey, oldestUse = key, entry.used
			}
		}
		if oldestKey == "" {
			return nil
		}
		delete(a.previews, oldestKey)
	}
	entry := &filePreview{thumbnail: thumbnail, pending: true, used: a.previewClock}
	select {
	case a.previewJobs <- previewJob{key: key, entry: entry, load: load}:
		a.previews[key] = entry
		return entry
	default:
		return nil
	}
}

func svgPreview(path string) ui.ImageSource {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return nil
	}
	if svg, err := ui.ParseSVG(data); err == nil {
		return svg
	}
	return nil
}
