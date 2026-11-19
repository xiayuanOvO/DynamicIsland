package lyrics

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCachePersistsLyricsWithoutNetwork(t *testing.T) {
	dataDir := t.TempDir()
	song := Song{Title: "夜行列车", Artist: "林海风", Album: "远方", Duration: 226}
	want := Lines{{At: 12.5, Text: "窗外亮起一盏灯"}, {At: 19}}
	cache := NewCache(dataDir)
	if err := cache.Save(song, want); err != nil {
		t.Fatal(err)
	}
	file, err := os.Stat(cache.path(song))
	if err != nil {
		t.Fatal(err)
	}
	cache = NewCache(dataDir)
	if size, err := cache.Size(); err != nil || size != file.Size() || size == 0 {
		t.Fatalf("cache size = %d, %v; want %d bytes", size, err, file.Size())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, downloaded := cache.Load(ctx, "", song); downloaded || !slices.Equal(got, want) {
		t.Fatalf("offline cache returned %v, downloaded = %v", got, downloaded)
	}
	other := song
	other.Duration += 20
	if got, _ := cache.Load(ctx, "", other); len(got) != 0 {
		t.Fatalf("different recording found cached lyrics: %v", got)
	}

	folder := filepath.Join(dataDir, "lyrics")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, song.Artist+" - "+song.Title+".lrc"), []byte("[00:14.00]云散了"), 0600); err != nil {
		t.Fatal(err)
	}
	local := Lines{{At: 14, Text: "云散了"}}
	if got, downloaded := cache.Load(ctx, folder, song); downloaded || !slices.Equal(got, local) {
		t.Fatalf("lookup with manual lyrics returned %v, downloaded = %v", got, downloaded)
	}
	if err := cache.Clear(); err != nil {
		t.Fatal(err)
	}
	if size, err := cache.Size(); err != nil || size != 0 {
		t.Fatalf("cache size after clearing = %d, %v; want 0 bytes", size, err)
	}
	if got, _ := cache.Load(ctx, "", song); len(got) != 0 {
		t.Fatalf("cleared cache still has lyrics: %v", got)
	}
	if got, _ := cache.Load(ctx, folder, song); !slices.Equal(got, local) {
		t.Fatalf("clearing affected manually added lyrics: %v", got)
	}
}
