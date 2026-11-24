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
	"unicode"
)

type Song struct {
	Title, Artist, Album string
	Duration             float64
}

// Load returns the lyrics and reports whether they came from an online source.
func (c Cache) Load(ctx context.Context, folder string, song Song) (lines Lines, downloaded bool) {
	if folder != "" {
		name := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_").
			Replace(song.Artist + " - " + song.Title)
		if file, err := os.Open(filepath.Join(folder, name+".lrc")); err == nil {
			data, err := io.ReadAll(io.LimitReader(file, 1<<20))
			file.Close()
			if err == nil {
				if lines := Parse(string(data)); len(lines) > 0 {
					return lines, false
				}
			}
		}
	}
	var cachedFallback Lines
	if data, err := os.ReadFile(c.path(song)); err == nil {
		var cached Lines
		if json.Unmarshal(data, &cached) == nil && len(cached) > 0 {
			if lyricsMatchSongLanguage(song, cached) {
				return cached, false
			}
			cachedFallback = cached
		}
	}
	if song.Artist == "" || ctx.Err() != nil {
		return cachedFallback, false
	}
	lines = loadOnline(ctx, http.DefaultClient, song)
	if len(lines) == 0 && len(cachedFallback) > 0 {
		return cachedFallback, false
	}
	return lines, len(lines) > 0
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func songExpectsHanLyrics(song Song) bool {
	if !containsHan(song.Title) {
		return false
	}
	lower := strings.ToLower(song.Title)
	return !strings.Contains(lower, "english") && !strings.Contains(song.Title, "英文")
}

func lyricsMatchSongLanguage(song Song, lines Lines) bool {
	if !songExpectsHanLyrics(song) {
		return true
	}
	for _, line := range lines {
		if containsHan(line.Text) {
			return true
		}
	}
	return false
}

func loadOnline(ctx context.Context, client *http.Client, song Song) Lines {
	sources := []func(context.Context, *http.Client, Song) Lines{
		func(ctx context.Context, client *http.Client, song Song) Lines {
			return search(ctx, client, "https://lrclib.net/api/search", song)
		},
		searchNetEase,
		searchQQ,
		searchKugou,
	}
	var fallback Lines
	for _, source := range sources {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		lines := source(attempt, client, song)
		cancel()
		if ctx.Err() != nil {
			if len(lines) > 0 && lyricsMatchSongLanguage(song, lines) {
				return lines
			}
			return fallback
		}
		if len(lines) == 0 {
			continue
		}
		if lyricsMatchSongLanguage(song, lines) {
			return lines
		}
		if len(fallback) == 0 {
			fallback = lines
		}
	}
	return fallback
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
	var difference float64
	if song.Duration > 0 && duration > 0 {
		difference = math.Abs(duration - song.Duration)
		if difference > 5 {
			return math.Inf(1)
		}
	}
	sameAlbum := song.Album != "" && strings.EqualFold(strings.TrimSpace(album), song.Album)
	sameRelease := sameAlbum && song.Duration > 0 && duration > 0 && difference <= 2
	artistScore := math.Inf(1)
	for _, artist := range artists {
		if strings.EqualFold(strings.TrimSpace(artist), song.Artist) {
			artistScore = 0
			break
		}
	}
	if !strings.EqualFold(strings.TrimSpace(title), song.Title) {
		if artistScore != 0 || !sameRelease {
			return math.Inf(1)
		}
		playing, playingLanguage := comparableTitle(song.Title)
		candidate, candidateLanguage := comparableTitle(title)
		if !strings.EqualFold(playing, candidate) || (playingLanguage != "" && candidateLanguage != "" && playingLanguage != candidateLanguage) {
			return math.Inf(1)
		}
		return 12 + difference // Prefer an exact title when both versions match.
	}
	if artistScore != 0 && sameRelease {
		// Providers can credit an alternative name on the identical release.
		artistScore = 8
	}
	if song.Album != "" && !sameAlbum {
		artistScore++
	}
	return artistScore + difference
}

var titleSpelling = strings.NewReplacer("妳", "你", "（", "(", "）", ")")

func comparableTitle(title string) (base, language string) {
	title = titleSpelling.Replace(strings.TrimSpace(title))
	if end, ok := strings.CutSuffix(title, ")"); ok {
		if i := strings.LastIndexByte(end, '('); i > 0 {
			switch strings.ToLower(strings.TrimSpace(end[i+1:])) {
			case "国", "國", "国语", "國語", "国语版", "國語版", "普通话", "普通話", "mandarin", "mandarin version":
				return strings.TrimSpace(end[:i]), "mandarin"
			case "粤", "粵", "粤语", "粵語", "粤语版", "粵語版", "cantonese", "cantonese version":
				return strings.TrimSpace(end[:i]), "cantonese"
			}
		}
	}
	return title, ""
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
