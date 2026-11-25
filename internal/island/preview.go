package island

import (
	"bytes"
	"image"
	"image/png"
	"strconv"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

const previewThumbnailSize = 256

type cachedPreview struct {
	source  ui.ImageSource
	pending bool
	used    uint64
}

type previewJob struct {
	key   string
	entry *cachedPreview
	load  func() ui.ImageSource
}

func (a *island) startPreviews() {
	a.previews = make(map[string]*cachedPreview)
	a.previewJobs = make(chan previewJob, 16)
	for range 2 {
		go func() {
			for job := range a.previewJobs {
				source := job.load()
				a.win.Update(func() {
					if a.previews[job.key] == job.entry {
						job.entry.source, job.entry.pending = source, false
					}
				})
			}
		}()
	}
}

func (a *island) clipboardPreview(id int64) *cachedPreview {
	store := a.clipboard.store
	return a.queuePreview("clipboard:"+strconv.FormatInt(id, 10), func() ui.ImageSource {
		var bitmap []byte
		if err := store.QueryRow("SELECT image FROM entries WHERE id=?", id).Scan(&bitmap); err != nil {
			return nil
		}
		if img, err := png.Decode(bytes.NewReader(bitmap)); err == nil {
			return ui.NewBitmap(clipboardThumbnail(img))
		}
		return nil
	})
}

func (a *island) queuePreview(key string, load func() ui.ImageSource) *cachedPreview {
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
	entry := &cachedPreview{pending: true, used: a.previewClock}
	select {
	case a.previewJobs <- previewJob{key: key, entry: entry, load: load}:
		a.previews[key] = entry
		return entry
	default:
		return nil
	}
}

func clipboardThumbnail(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if longest := max(w, h); longest > previewThumbnailSize {
		w, h = max(1, w*previewThumbnailSize/longest), max(1, h*previewThumbnailSize/longest)
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), img, bounds, draw.Src, nil)
	return thumb
}
