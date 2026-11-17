package shelf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAddAndRemovePreserveOriginals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "中文 文件 #1.txt")
	content := []byte("keep the original")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	items, rejected := Inspect([]string{path, path, dir, filepath.Join(dir, "missing"), ""})
	if rejected != 2 {
		t.Fatalf("rejected = %d, want 2", rejected)
	}
	s := &Shelf{}
	report := s.Add(items)
	if report.Added != 2 || report.Duplicates != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if s.Items[0].Directory || !s.Items[1].Directory {
		t.Fatal("files and folders must retain their types")
	}
	if again := s.Add(items); again.Added != 0 || again.Duplicates != 3 {
		t.Fatalf("repeated drop changed the shelf: %+v", again)
	}
	s.Remove(s.Paths())
	if len(s.Items) != 0 {
		t.Fatal("shelf was not cleared")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(content) {
		t.Fatalf("original was changed: %q, %v", got, err)
	}
}

func TestSaveReplacesListAndRestoreSkipsMissing(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(dir, "config", "shelf.json")
	s := &Shelf{}
	items, _ := Inspect([]string{first, second})
	s.Add(items[:1])
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	s.Add(items[1:])
	if err := s.Save(state); err != nil {
		t.Fatalf("replace existing list: %v", err)
	}
	// Removing a source outside the shelf simulates a moved/deleted file.
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	loaded, skipped, err := Load(state)
	if err != nil || skipped != 1 || len(loaded.Items) != 1 || loaded.Items[0].Path != second {
		t.Fatalf("restore = %+v, skipped %d, error %v", loaded, skipped, err)
	}
	s.Remove(s.Paths())
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = Load(state)
	if err != nil || len(loaded.Items) != 0 {
		t.Fatalf("empty list was not persisted: %+v, %v", loaded, err)
	}
}

func TestLoadMissingAndMalformedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shelf.json")
	s, _, err := Load(path)
	if err != nil || len(s.Items) != 0 {
		t.Fatalf("first launch: %+v, %v", s, err)
	}
	if err := os.WriteFile(path, []byte("broken json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); err == nil {
		t.Fatal("malformed state must be reported")
	}
}

func TestWindowsPathsIgnoreCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path semantics")
	}
	s := &Shelf{}
	report := s.Add([]Item{
		{Path: `C:\Files\Document.txt`},
		{Path: `c:\files\DOCUMENT.TXT`},
	})
	if report.Added != 1 || report.Duplicates != 1 {
		t.Fatalf("case-insensitive duplicate: %+v", report)
	}
	s.Remove([]string{`c:\FILES\document.txt`})
	if len(s.Items) != 0 {
		t.Fatal("case-insensitive removal failed")
	}
}
