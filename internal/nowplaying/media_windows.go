package nowplaying

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

//go:embed session_windows.ps1
var sessionScript string

var encodedScript = encodePowerShell(sessionScript)

func encodePowerShell(script string) string {
	chars := utf16.Encode([]rune(script))
	buf := make([]byte, len(chars)*2)
	for i, char := range chars {
		binary.LittleEndian.PutUint16(buf[i*2:], char)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func Adapter() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

type windowsWorkerRequest struct {
	Op       string `json:"op"`
	Artwork  bool   `json:"artwork,omitempty"`
	Action   string `json:"action,omitempty"`
	Source   string `json:"source,omitempty"`
	Position int64  `json:"position,omitempty"`
}

type windowsMediaWorker struct {
	mu      sync.Mutex
	adapter string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
}

var (
	defaultWindowsWorker windowsMediaWorker
	windowsWatchWake     = make(chan struct{}, 1)
)

func (w *windowsMediaWorker) startLocked(adapter string) error {
	if w.cmd != nil && w.adapter == adapter {
		return nil
	}
	w.closeLocked()
	cmd := exec.Command(adapter, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return err
	}
	w.adapter = adapter
	w.cmd = cmd
	w.stdin = stdin
	w.stdout = bufio.NewReaderSize(stdout, 64*1024)
	return nil
}

func (w *windowsMediaWorker) closeLocked() {
	if w.stdin != nil {
		_ = w.stdin.Close()
		w.stdin = nil
	}
	if w.cmd != nil {
		if w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
		}
		_ = w.cmd.Wait()
		w.cmd = nil
	}
	w.stdout = nil
	w.adapter = ""
}

func (w *windowsMediaWorker) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closeLocked()
}

func (w *windowsMediaWorker) request(ctx context.Context, adapter string, req windowsWorkerRequest) ([]byte, error) {
	if adapter == "" {
		return nil, errors.New("missing media adapter")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.startLocked(adapter); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	if _, err := w.stdin.Write(payload); err != nil {
		w.closeLocked()
		return nil, err
	}

	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	reader := w.stdout
	go func() {
		line, readErr := reader.ReadBytes('\n')
		ch <- readResult{line: line, err: readErr}
	}()

	select {
	case <-ctx.Done():
		w.closeLocked()
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			w.closeLocked()
			return nil, res.err
		}
		line := bytes.TrimSpace(res.line)
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(line, &errResp) == nil && errResp.Error != "" {
			return nil, errors.New(errResp.Error)
		}
		return line, nil
	}
}

func Read(ctx context.Context, adapter string, withArtwork bool) (Track, error) {
	data, err := defaultWindowsWorker.request(ctx, adapter, windowsWorkerRequest{
		Op:      "read",
		Artwork: withArtwork,
	})
	if err != nil {
		return Track{}, err
	}
	track, err := decodeWindowsTrack(data)
	if err != nil {
		return Track{}, err
	}
	track.Sampled = time.Now()
	if track.IsNetEase() && track.Title != "" && track.Artist != "" && !track.HasPosition() {
		uptime := windows.DurationSinceBoot()
		track.Position, track.Duration = readNetEaseTimeline(track, uptime)
		if track.Duration > 0 && track.Rate <= 0 {
			track.Rate = 1
		}
	}
	return track, nil
}

func decodeWindowsTrack(data []byte) (Track, error) {
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		return Track{}, err
	}
	// Apple Music can publish "Artist — Album" as the artist and leave the album empty.
	if strings.EqualFold(track.Source, "AppleInc.AppleMusicWin_nzyj5cx40ttqa!App") && strings.TrimSpace(track.Album) == "" {
		artist, album, _ := strings.Cut(track.Artist, " — ")
		artist, album = strings.TrimSpace(artist), strings.TrimSpace(album)
		if artist != "" && album != "" {
			track.Artist, track.Album = artist, album
		}
	}
	return track, nil
}

func Control(ctx context.Context, adapter string, command Command) error {
	data, err := defaultWindowsWorker.request(ctx, adapter, windowsWorkerRequest{
		Op:       "control",
		Action:   string(command.Action),
		Source:   command.Source,
		Position: int64(command.Position * 1e7),
	})
	if err != nil {
		return err
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || !resp.OK {
		return errors.New("media control rejected")
	}
	select {
	case windowsWatchWake <- struct{}{}:
	default:
	}
	return nil
}

func Watch(ctx context.Context, adapter string, onTrack func(Track)) {
	if adapter == "" {
		return
	}
	defer Close()

	const (
		activeInterval = 250 * time.Millisecond
		idleInterval   = 500 * time.Millisecond
		minBackoff     = 500 * time.Millisecond
		maxBackoff     = 10 * time.Second
	)

	var (
		lastTrack       Track
		hasLast         bool
		backoff         = minBackoff
		lastTick        = time.Now()
		expectedWait    = activeInterval
		fastUntil       time.Time
	)

	for ctx.Err() == nil {
		now := time.Now()
		if sleepWakeDetected(lastTick, now, expectedWait) {
			defaultWindowsWorker.close()
			hasLast = false
		}
		lastTick = now

		needArtwork := !hasLast || lastTrack.Title == ""
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		track, err := Read(reqCtx, adapter, needArtwork)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("windows media worker read: %v", err)
			expectedWait = backoff
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-windowsWatchWake:
				timer.Stop()
				backoff = minBackoff
			case <-timer.C:
				backoff = min(backoff*2, maxBackoff)
			}
			continue
		}
		backoff = minBackoff

		// If the song changed and we didn't request artwork on this poll, fetch artwork once.
		if track.Title != "" && !needArtwork && !lastTrack.SameSong(track) {
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if withArt, artErr := Read(reqCtx, adapter, true); artErr == nil {
				track = withArt
			}
			cancel()
		}

		if !hasLast || !windowsTrackEquivalent(lastTrack, track) {
			onTrack(track)
			lastTrack = track
			lastTrack.Artwork = nil
			hasLast = true
		}

		wait := idleInterval
		if track.Playing || time.Now().Before(fastUntil) {
			wait = activeInterval
		}
		expectedWait = wait
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-windowsWatchWake:
			timer.Stop()
			fastUntil = time.Now().Add(1500 * time.Millisecond)
		case <-timer.C:
		}
	}
}

func Close() {
	defaultWindowsWorker.close()
}

func windowsTrackEquivalent(a, b Track) bool {
	if a.Source != b.Source || a.ID != b.ID || a.Title != b.Title || a.Artist != b.Artist || a.Album != b.Album ||
		a.Playing != b.Playing || a.Previous != b.Previous || a.Next != b.Next || a.Toggle != b.Toggle || a.Seek != b.Seek ||
		a.Rate != b.Rate || a.Duration != b.Duration {
		return false
	}
	if a.Title == "" {
		return true
	}
	expected := a.Elapsed(b.Sampled)
	return math.Abs(expected-b.Position) < 1.5
}
