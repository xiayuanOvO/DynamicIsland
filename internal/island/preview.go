package island

import (
	"io"
	"os"
	"path/filepath"
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
	path, key string
	entry     *filePreview
}

func (a *island) startPreviews() {
	a.previews = make(map[string]*filePreview)
	a.previewJobs = make(chan previewJob, 16)
	for range 2 {
		go func() {
			for job := range a.previewJobs {
				var source ui.ImageSource
				if img, err := preview.Load(job.path, job.entry.thumbnail); err == nil {
					source = ui.NewBitmap(img)
				} else if job.entry.thumbnail && strings.EqualFold(filepath.Ext(job.path), ".svg") {
					source = svgPreview(job.path)
				}
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
	key := shelf.Key(item.Path)
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
	entry := &filePreview{
		thumbnail: !item.Directory && preview.IsImage(item.Path), pending: true, used: a.previewClock,
	}
	select {
	case a.previewJobs <- previewJob{path: item.Path, key: key, entry: entry}:
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
