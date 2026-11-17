package shelf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Item refers to an existing file or directory; the shelf never owns its contents.
type Item struct {
	Path      string
	Name      string
	Size      int64
	Directory bool
}

type Shelf struct {
	Items []Item
}

type Report struct {
	Added      int
	Duplicates int
	Rejected   int
}

func Key(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// Inspect runs independently of the UI so slow disks do not block a drop.
func Inspect(paths []string) ([]Item, int) {
	items := make([]Item, 0, len(paths))
	rejected := 0
	for _, path := range paths {
		if path == "" {
			rejected++
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			rejected++
			continue
		}
		info, err := os.Stat(absolute)
		if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			rejected++
			continue
		}
		items = append(items, Item{
			Path: absolute, Name: info.Name(), Size: info.Size(), Directory: info.IsDir(),
		})
	}
	return items, rejected
}

func (s *Shelf) Add(items []Item) Report {
	seen := make(map[string]bool, len(s.Items)+len(items))
	for _, item := range s.Items {
		seen[Key(item.Path)] = true
	}
	var report Report
	for _, item := range items {
		key := Key(item.Path)
		if seen[key] {
			report.Duplicates++
			continue
		}
		seen[key] = true
		s.Items = append(s.Items, item)
		report.Added++
	}
	return report
}

func (s *Shelf) Remove(paths []string) {
	remove := make(map[string]bool, len(paths))
	for _, path := range paths {
		remove[Key(path)] = true
	}
	s.Items = slices.DeleteFunc(s.Items, func(item Item) bool { return remove[Key(item.Path)] })
}

func (s *Shelf) Paths() []string {
	paths := make([]string, 0, len(s.Items))
	for _, item := range s.Items {
		paths = append(paths, item.Path)
	}
	return paths
}

func Load(path string) (*Shelf, int, error) {
	s := &Shelf{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, 0, nil
	}
	if err != nil {
		return s, 0, err
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return s, 0, fmt.Errorf("读取暂存列表: %w", err)
	}
	items, rejected := Inspect(paths)
	s.Add(items)
	return s, rejected, nil
}

// Save replaces the list through a temporary file, never touching source files.
func (s *Shelf) Save(path string) error {
	data, err := json.MarshalIndent(s.Paths(), "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".shelf-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
