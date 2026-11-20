package nowplaying

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

func Read(ctx context.Context, adapter string, withArtwork bool) (Track, error) {
	data, err := run(ctx, adapter, Command{}, withArtwork)
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
	_, err := run(ctx, adapter, command, false)
	return err
}

func run(ctx context.Context, adapter string, command Command, withArtwork bool) ([]byte, error) {
	cmd := exec.CommandContext(ctx, adapter, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Env = append(os.Environ(),
		"DYNAMICISLAND_MEDIA_ACTION="+string(command.Action),
		"DYNAMICISLAND_MEDIA_ARTWORK="+strconv.FormatBool(withArtwork),
		"DYNAMICISLAND_MEDIA_SOURCE="+command.Source,
		"DYNAMICISLAND_MEDIA_POSITION="+strconv.FormatInt(int64(command.Position*1e7), 10),
	)
	return cmd.Output()
}
