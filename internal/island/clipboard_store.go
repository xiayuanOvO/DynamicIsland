package island

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	clipboardHistoryLimit = 10_000
	clipboardRetention    = 90 * 24 * time.Hour
)

type clipboardStore struct{ *sql.DB }

type clipboardRecord struct {
	text          string
	png           []byte
	width, height int
	copiedAt      time.Time
}

const clipboardSettingsSQL = `INSERT INTO settings (id, recording, shortcut)
	VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET recording=excluded.recording, shortcut=excluded.shortcut`

func openClipboardStore(path string) (*clipboardStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	dsnPath := filepath.ToSlash(path)
	if !strings.HasPrefix(dsnPath, "/") {
		dsnPath = "/" + dsnPath
	}
	dsn := (&url.URL{Scheme: "file", Path: dsnPath, RawQuery: "_pragma=busy_timeout(3000)&_pragma=secure_delete(ON)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA auto_vacuum=FULL;
		CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK(id=1), recording INTEGER NOT NULL, shortcut TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			copied_at INTEGER NOT NULL,
			preview TEXT NOT NULL,
			image_width INTEGER NOT NULL,
			image_height INTEGER NOT NULL,
			content_key BLOB NOT NULL UNIQUE,
			search_text TEXT NOT NULL,
			text TEXT NOT NULL,
			image BLOB,
			pinned INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS entries_recent ON entries (copied_at DESC, id DESC);
	`); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureClipboardPinnedColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS entries_pinned_recent ON entries (pinned DESC, copied_at DESC, id DESC)`); err != nil {
		db.Close()
		return nil, err
	}
	return &clipboardStore{DB: db}, nil
}

func ensureClipboardPinnedColumn(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(entries)")
	if err != nil {
		return err
	}
	hasPinned := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "pinned" {
			hasPinned = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if hasPinned {
		return nil
	}
	_, err = db.Exec("ALTER TABLE entries ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0")
	return err
}

func (s *clipboardStore) settings() (clipboardState, error) {
	var settings clipboardState
	err := s.QueryRow("SELECT recording, shortcut FROM settings WHERE id=1").Scan(&settings.Recording, &settings.Shortcut)
	return settings, err
}

func (s *clipboardStore) saveSettings(settings clipboardState) error {
	_, err := s.Exec(clipboardSettingsSQL, settings.Recording, settings.Shortcut)
	return err
}

func (s *clipboardStore) record(record clipboardRecord) (bool, error) {
	tx, err := s.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := storeClipboardRecord(tx, record)
	if err != nil {
		return false, err
	}
	pruned, err := pruneClipboardRecords(tx, record.copiedAt)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0 || pruned, tx.Commit()
}

func storeClipboardRecord(tx *sql.Tx, record clipboardRecord) (sql.Result, error) {
	hash := sha256.New()
	hash.Write([]byte(record.text))
	hash.Write([]byte{0})
	hash.Write(record.png)
	searchText := strings.ToLower(record.text)
	if len(record.png) > 0 {
		searchText += " 图片"
	}
	preview := strings.TrimSpace(record.text)
	length := 0
	for i, r := range preview {
		if length == 120 || r == '\r' || r == '\n' {
			preview = preview[:i] + "…"
			break
		}
		length++
	}
	return tx.Exec(`INSERT INTO entries (copied_at, preview, image_width, image_height, content_key, search_text, text, image)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(content_key) DO UPDATE SET copied_at=excluded.copied_at
		WHERE excluded.copied_at > entries.copied_at`, record.copiedAt.UnixNano(), preview, record.width, record.height,
		hash.Sum(nil), searchText, record.text, record.png)
}

func pruneClipboardRecords(tx *sql.Tx, now time.Time) (bool, error) {
	expired, err := tx.Exec("DELETE FROM entries WHERE pinned = 0 AND copied_at <= ?", now.Add(-clipboardRetention).UnixNano())
	if err != nil {
		return false, err
	}
	overflow, err := tx.Exec(`DELETE FROM entries WHERE pinned = 0 AND id IN (
		SELECT id FROM entries ORDER BY pinned DESC, copied_at DESC, id DESC LIMIT -1 OFFSET ?
	)`, clipboardHistoryLimit)
	if err != nil {
		return false, err
	}
	n, err := expired.RowsAffected()
	if err != nil {
		return false, err
	}
	m, err := overflow.RowsAffected()
	return n+m > 0, err
}

func (s *clipboardStore) expire(now time.Time) (bool, error) {
	result, err := s.Exec("DELETE FROM entries WHERE pinned = 0 AND copied_at <= ?", now.Add(-clipboardRetention).UnixNano())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *clipboardStore) entryPage(query string, now time.Time, after clipboardEntry, limit int) ([]clipboardEntry, bool, error) {
	statement := "SELECT id, preview, copied_at, image_width, image_height, pinned FROM entries WHERE (pinned = 1 OR copied_at > ?)"
	args := []any{now.Add(-clipboardRetention).UnixNano()}
	if after.ID != 0 {
		afterPinned := 0
		if after.Pinned {
			afterPinned = 1
		}
		statement += " AND (pinned, copied_at, id) < (?, ?, ?)"
		args = append(args, afterPinned, after.CopiedAt.UnixNano(), after.ID)
	}
	if terms := strings.Fields(strings.ToLower(query)); len(terms) > 0 {
		encoded, _ := json.Marshal(terms)
		statement += " AND NOT EXISTS (SELECT 1 FROM json_each(?) WHERE instr(entries.search_text, value) = 0)"
		args = append(args, string(encoded))
	}
	rows, err := s.Query(statement+" ORDER BY pinned DESC, copied_at DESC, id DESC LIMIT ?", append(args, limit+1)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var entries []clipboardEntry
	for rows.Next() {
		if len(entries) == limit {
			return entries, true, nil
		}
		var entry clipboardEntry
		var copiedAt int64
		var pinned int
		if err := rows.Scan(&entry.ID, &entry.Preview, &copiedAt, &entry.Width, &entry.Height, &pinned); err != nil {
			return nil, false, err
		}
		entry.CopiedAt = time.Unix(0, copiedAt)
		entry.Pinned = pinned != 0
		entries = append(entries, entry)
	}
	return entries, false, rows.Err()
}

func (s *clipboardStore) setPinned(id int64, pinned bool) error {
	value := 0
	if pinned {
		value = 1
	}
	_, err := s.Exec("UPDATE entries SET pinned=? WHERE id=?", value, id)
	return err
}

func (s *clipboardStore) deleteEntry(id int64) error {
	_, err := s.Exec("DELETE FROM entries WHERE id=?", id)
	return err
}

func (s *clipboardStore) clear() error {
	_, err := s.Exec("DELETE FROM entries")
	return err
}

func (s *clipboardStore) migrate(path, imageDir string, now time.Time) (bool, error) {
	var legacy struct {
		clipboardState
		Entries []struct {
			Text     string    `json:"text"`
			CopiedAt time.Time `json:"copiedAt"`
			Image    *struct {
				ID     string `json:"id"`
				Size   int    `json:"size"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			} `json:"image"`
		} `json:"entries"`
	}
	data, err := os.ReadFile(path)
	found := err == nil
	if found {
		err = json.Unmarshal(data, &legacy)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	tx, err := s.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Legacy entries are newest first. Insert in reverse to preserve equal timestamps.
	for i := len(legacy.Entries) - 1; i >= 0; i-- {
		entry := legacy.Entries[i]
		if !entry.CopiedAt.After(now.Add(-clipboardRetention)) {
			continue
		}
		record := clipboardRecord{copiedAt: entry.CopiedAt}
		if validClipboardText(entry.Text) {
			record.text = entry.Text
		}
		if img := entry.Image; img != nil {
			id, err := hex.DecodeString(img.ID)
			if err != nil || len(id) != sha256.Size || img.Size <= 0 || img.Size > clipboardImageLimit ||
				!validClipboardDimensions(img.Width, img.Height) {
				return false, fmt.Errorf("invalid legacy clipboard image at entry %d", i+1)
			}
			path := filepath.Join(imageDir, img.ID+".png")
			file, err := os.Open(path)
			if errors.Is(err, os.ErrNotExist) {
				log.Printf("skip missing legacy clipboard image at entry %d", i+1)
			} else if err != nil {
				return false, err
			} else {
				pngBytes, err := io.ReadAll(io.LimitReader(file, int64(img.Size)+1))
				file.Close()
				digest := sha256.Sum256(pngBytes)
				if err != nil || len(pngBytes) != img.Size || !bytes.Equal(digest[:], id) {
					return false, fmt.Errorf("unreadable legacy clipboard image at entry %d", i+1)
				}
				record.png, record.width, record.height = pngBytes, img.Width, img.Height
			}
		}
		if record.text != "" || len(record.png) > 0 {
			if _, err := storeClipboardRecord(tx, record); err != nil {
				return false, err
			}
		}
	}
	if _, err := pruneClipboardRecords(tx, now); err != nil {
		return false, err
	}
	if _, err := tx.Exec(clipboardSettingsSQL, legacy.Recording, legacy.Shortcut); err != nil {
		return false, err
	}
	return found, tx.Commit()
}
