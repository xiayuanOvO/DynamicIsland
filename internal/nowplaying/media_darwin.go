package nowplaying

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

var (
	selSharedWorkspace     = objc.RegisterName("sharedWorkspace")
	selRunningApplications = objc.RegisterName("runningApplications")
	selBundleIdentifier    = objc.RegisterName("bundleIdentifier")
	selIsEqualToString     = objc.RegisterName("isEqualToString:")
	selArrayCount          = objc.RegisterName("count")
	selObjectAtIndex       = objc.RegisterName("objectAtIndex:")
	selAlloc               = objc.RegisterName("alloc")
	selInit                = objc.RegisterName("init")
	selDrain               = objc.RegisterName("drain")

	musicBundleCF objc.ID

	// isMusicRunning is a variable so tests can stub NSWorkspace checks.
	isMusicRunning = isAppleMusicRunning
)

func init() {
	_, _ = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return
	}
	var stringCF func(uintptr, string, uint32) objc.ID
	purego.RegisterLibFunc(&stringCF, cf, "CFStringCreateWithCString")
	const utf8 = 0x08000100
	musicBundleCF = stringCF(0, "com.apple.Music", utf8)
}

func isAppleMusicRunning() bool {
	if musicBundleCF == 0 {
		return false
	}
	wsClass := objc.GetClass("NSWorkspace")
	if wsClass == 0 {
		return false
	}
	poolClass := objc.GetClass("NSAutoreleasePool")
	if poolClass != 0 {
		pool := objc.ID(poolClass).Send(selAlloc).Send(selInit)
		defer pool.Send(selDrain)
	}
	workspace := objc.ID(wsClass).Send(selSharedWorkspace)
	if workspace == 0 {
		return false
	}
	apps := workspace.Send(selRunningApplications)
	if apps == 0 {
		return false
	}
	count := objc.Send[uint](apps, selArrayCount)
	for i := range count {
		app := apps.Send(selObjectAtIndex, i)
		if app == 0 {
			continue
		}
		bid := app.Send(selBundleIdentifier)
		if bid != 0 && objc.Send[bool](bid, selIsEqualToString, musicBundleCF) {
			return true
		}
	}
	return false
}

func Adapter() string {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return ""
	}
	adapter, _ := exec.LookPath(filepath.Join(resources, "media-control", "bin", "media-control"))
	return adapter
}

//go:embed music_darwin.js
var musicScript string

//go:embed artwork_darwin.applescript
var musicArtworkScript string

const musicScriptSource = "applescript:com.apple.Music"

func Read(ctx context.Context, adapter string, withArtwork bool) (Track, error) {
	track, hasTimeline, err := readSystemMedia(ctx, adapter, withArtwork)
	resolved := resolveDarwinTrack(ctx, track, hasTimeline)
	if resolved.Title != "" {
		return resolved, nil
	}
	return resolved, err
}

func resolveDarwinTrack(ctx context.Context, track Track, hasTimeline bool) Track {
	isMusic := track.Source == "com.apple.Music"
	if track.Title != "" && track.Playing && (!isMusic || hasTimeline) {
		if isMusic && len(track.Artwork) == 0 {
			track.ArtworkLoader = musicArtworkLoader("", track)
		}
		return track
	}
	if !isMusicRunning() {
		return track
	}
	data, musicErr := runMusicScript(ctx)
	if musicErr == nil && len(data) > 0 {
		var music Track
		if json.Unmarshal(data, &music) == nil && music.Title != "" {
			music.Source = musicScriptSource
			music.Sampled = time.Now()
			music.Rate = 1
			music.Previous, music.Next, music.Toggle, music.Seek = true, true, true, true
			if isMusic && track.Title == music.Title && track.Artist == music.Artist {
				music.Artwork = track.Artwork
			}
			if music.ID != "" {
				music.ArtworkLoader = musicArtworkLoader(music.ID, music)
			}
			return music
		}
	}
	return track
}

func Watch(ctx context.Context, adapter string, onTrack func(Track)) {
	if adapter == "" {
		return
	}
	const (
		minBackoff = 500 * time.Millisecond
		maxBackoff = 10 * time.Second
	)
	backoff := minBackoff
	for ctx.Err() == nil {
		started := time.Now()
		gotEvent, wakeRestart, err := runDarwinStream(ctx, adapter, onTrack)
		if ctx.Err() != nil {
			return
		}
		if wakeRestart {
			backoff = minBackoff
			continue
		}
		if err != nil {
			log.Printf("media stream exited: %v", err)
		}
		if gotEvent || time.Since(started) > maxBackoff {
			backoff = minBackoff
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func Close() {}

func runDarwinStream(ctx context.Context, adapter string, onTrack func(Track)) (gotEvent bool, wakeRestart bool, err error) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	cmd := exec.Command(adapter, "stream", "--no-diff", "--debounce=50")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, false, err
	}
	if err := cmd.Start(); err != nil {
		return false, false, err
	}

	var woke atomic.Bool
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-streamCtx.Done():
			killProcessGroup(cmd)
			_ = stdout.Close()
		case <-done:
		}
	}()

	go func() {
		const checkInterval = 30 * time.Second
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		lastTick := time.Now()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				if sleepWakeDetected(lastTick, now, checkInterval) {
					woke.Store(true)
					cancelStream()
					return
				}
				lastTick = now
			}
		}
	}()

	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var event struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(line, &event) == nil && event.Type == "data" && len(event.Payload) > 0 {
				if track, hasTimeline, decodeErr := decodeSystemMediaPayload(event.Payload); decodeErr == nil {
					gotEvent = true
					resolved := resolveDarwinTrack(streamCtx, track, hasTimeline)
					if streamCtx.Err() == nil {
						onTrack(resolved)
					}
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				readErr = nil
			}
			cancelStream()
			killProcessGroup(cmd)
			waitErr := cmd.Wait()
			if readErr != nil {
				return gotEvent, woke.Load(), readErr
			}
			return gotEvent, woke.Load(), waitErr
		}
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}

func musicArtworkLoader(persistentID string, track Track) func(context.Context) []byte {
	args := []string{"-s", "s", "-", persistentID, track.Title, track.Artist, track.Album}
	return func(ctx context.Context) []byte {
		cmd := exec.CommandContext(ctx, "/usr/bin/osascript", args...)
		cmd.Stdin = strings.NewReader(musicArtworkScript)
		data, err := cmd.Output()
		raw, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "«data ")
		if err != nil || !ok || len(raw) <= 6 || len(raw) > (16<<20)+6 || !strings.HasSuffix(raw, "»") {
			return nil
		}
		artwork, _ := hex.DecodeString(raw[4 : len(raw)-len("»")])
		return artwork
	}
}

func readSystemMedia(ctx context.Context, adapter string, withArtwork bool) (Track, bool, error) {
	args := []string{"get", "--now"}
	if !withArtwork {
		args = append(args, "--no-artwork")
	}
	data, err := exec.CommandContext(ctx, adapter, args...).Output()
	if err != nil {
		return Track{}, false, err
	}
	return decodeSystemMediaPayload(data)
}

func decodeSystemMediaPayload(data []byte) (Track, bool, error) {
	var payload struct {
		BundleIdentifier, ContentItemIdentifier, Title, Artist, Album string
		ArtworkData                                                   []byte
		Duration                                                      float64
		ElapsedTime                                                   *float64
		ElapsedTimeNow, PlaybackRate                                  *float64
		Playing                                                       bool
		Timestamp                                                     string
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Track{}, false, err
	}
	sampled := time.Now()
	var position float64
	var hasElapsed bool
	if payload.ElapsedTimeNow != nil {
		position = *payload.ElapsedTimeNow
		hasElapsed = true
	} else if payload.ElapsedTime != nil {
		position = *payload.ElapsedTime
		hasElapsed = true
		if payload.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, payload.Timestamp); err == nil {
				sampled = ts
			}
		}
	}
	rate := 1.0
	if payload.PlaybackRate != nil {
		rate = *payload.PlaybackRate
	}
	return Track{
		Source: payload.BundleIdentifier, ID: payload.ContentItemIdentifier,
		Title: payload.Title, Artist: payload.Artist, Album: payload.Album,
		Artwork: payload.ArtworkData, Playing: payload.Playing, Position: position,
		Duration: payload.Duration, Rate: rate, Sampled: sampled,
		Previous: true, Next: true, Toggle: true, Seek: true,
	}, hasElapsed && payload.Duration > 0, nil
}

func Control(ctx context.Context, adapter string, command Command) error {
	if command.Source == musicScriptSource {
		_, err := runMusicScript(ctx, string(command.Action), strconv.FormatFloat(command.Position, 'f', 3, 64))
		return err
	}
	args := []string{string(command.Action)}
	if command.Action == Seek {
		args = append(args, "--micros", strconv.FormatInt(int64(command.Position*1e6), 10))
	}
	return exec.CommandContext(ctx, adapter, args...).Run()
}

func runMusicScript(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", append([]string{"-l", "JavaScript", "-"}, args...)...)
	cmd.Stdin = strings.NewReader(musicScript)
	return cmd.Output()
}
