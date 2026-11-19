package lyrics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Song struct {
	Title, Artist, Album string
	Duration             float64
}

func Load(ctx context.Context, folder string, song Song) Lines {
	if folder != "" {
		name := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").
			Replace(song.Artist + " - " + song.Title)
		if file, err := os.Open(filepath.Join(folder, name+".lrc")); err == nil {
			data, err := io.ReadAll(io.LimitReader(file, 1<<20))
			file.Close()
			if err == nil {
				if lines := Parse(string(data)); len(lines) > 0 {
					return lines
				}
			}
		}
	}
	if song.Artist == "" || ctx.Err() != nil {
		return nil
	}
	return loadOnline(ctx, http.DefaultClient, song)
}

func loadOnline(ctx context.Context, client *http.Client, song Song) Lines {
	for _, source := range []func(context.Context, *http.Client, Song) Lines{
		func(ctx context.Context, client *http.Client, song Song) Lines {
			return search(ctx, client, "https://lrclib.net/api/search", song)
		},
		searchNetEase,
		searchQQ,
		searchKugou,
	} {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		lines := source(attempt, client, song)
		cancel()
		if len(lines) > 0 || ctx.Err() != nil {
			return lines
		}
	}
	return nil
}

func search(ctx context.Context, client *http.Client, endpoint string, song Song) Lines {
	params := url.Values{"track_name": {song.Title}, "artist_name": {song.Artist}}
	var matches []struct {
		TrackName, ArtistName, AlbumName, SyncedLyrics string
		Duration                                       float64
	}
	if !fetchJSON(ctx, client, endpoint+"?"+params.Encode(), "", nil, &matches) {
		return nil
	}
	var best Lines
	bestScore := math.Inf(1)
	for _, match := range matches {
		score := matchScore(song, match.TrackName, []string{match.ArtistName}, match.AlbumName, match.Duration)
		if score < bestScore {
			if lines := Parse(match.SyncedLyrics); len(lines) > 0 {
				best, bestScore = lines, score
			}
		}
	}
	return best
}

func matchScore(song Song, title string, artists []string, album string, duration float64) float64 {
	if !strings.EqualFold(strings.TrimSpace(title), song.Title) {
		return math.Inf(1)
	}
	var difference float64
	if song.Duration > 0 && duration > 0 {
		difference = math.Abs(duration - song.Duration)
		if difference > 5 {
			return math.Inf(1)
		}
	}
	sameAlbum := song.Album != "" && strings.EqualFold(strings.TrimSpace(album), song.Album)
	artistScore := math.Inf(1)
	for _, artist := range artists {
		if strings.EqualFold(strings.TrimSpace(artist), song.Artist) {
			artistScore = 0
			break
		}
	}
	if artistScore != 0 && sameAlbum && song.Duration > 0 && duration > 0 && difference <= 2 {
		// Providers can credit an alternative name on the identical release.
		artistScore = 8
	}
	if song.Album != "" && !sameAlbum {
		artistScore++
	}
	return artistScore + difference
}

func fetchJSON(ctx context.Context, client *http.Client, address, referer string, body []byte, out any) bool {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "DynamicIsland/0.1.0")
	req.Header.Set("Accept", "application/json")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out) == nil
}
