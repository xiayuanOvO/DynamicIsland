package lyrics

import (
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestMatchScoreAllowsTitleVariantsOnSameRelease(t *testing.T) {
	song := Song{Title: "妳看见那盏灯吗 (国语)", Artist: "林海风", Album: "远方", Duration: 254}
	for _, tt := range []struct {
		name, title, artist, album string
		duration                   float64
		matches                    bool
	}{
		{"alternate spelling", "你看见那盏灯吗", song.Artist, song.Album, 254.248, true},
		{"translated language tag", "你看见那盏灯吗（Mandarin）", song.Artist, song.Album, 254.248, true},
		{"different language", "你看见那盏灯吗（粤语）", song.Artist, song.Album, 254.248, false},
		{"live recording", "你看见那盏灯吗 (Live)", song.Artist, song.Album, 254.248, false},
		{"different artist", "你看见那盏灯吗", "江雁", song.Album, 254.248, false},
		{"different album", "你看见那盏灯吗", song.Artist, "远方现场", 254.248, false},
		{"unknown album", "你看见那盏灯吗", song.Artist, "", 254.248, false},
		{"different duration", "你看见那盏灯吗", song.Artist, song.Album, 251, false},
		{"unknown duration", "你看见那盏灯吗", song.Artist, song.Album, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			score := matchScore(song, tt.title, []string{tt.artist}, tt.album, tt.duration)
			if matched := !math.IsInf(score, 1); matched != tt.matches {
				t.Errorf("score = %v, want match = %v", score, tt.matches)
			}
		})
	}
	if exact, variant := matchScore(song, song.Title, []string{song.Artist}, song.Album, 255),
		matchScore(song, "你看见那盏灯吗", []string{song.Artist}, song.Album, 254); exact >= variant {
		t.Errorf("exact title scored %v, variant scored %v", exact, variant)
	}
}

func TestNetEaseLoadsLyricsForTitleVariant(t *testing.T) {
	song := Song{Title: "妳看见那盏灯吗 (国语)", Artist: "林海风", Album: "远方", Duration: 254}
	client := &http.Client{Transport: lyricTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/api/search/get/web":
			body = `{"code":200,"result":{"songs":[
				{"id":1,"name":"你看见那盏灯吗","duration":236802,"album":{"name":"远方"},"artists":[{"name":"林海风"}]},
				{"id":2,"name":"你看见那盏灯吗","duration":254000,"album":{"name":"远方现场"},"artists":[{"name":"林海风"}]},
				{"id":3,"name":"你看见那盏灯吗","duration":254248,"album":{"name":"远方"},"artists":[{"name":"林海风"}]}
			]}}`
		case "/api/song/lyric":
			if id := r.URL.Query().Get("id"); id != "3" {
				t.Fatalf("downloaded lyrics for track %s, want 3", id)
			}
			body = `{"code":200,"lrc":{"lyric":"[01:52.00]远处那盏灯亮着"}}`
		default:
			t.Fatalf("unexpected query: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	lines := searchNetEase(t.Context(), client, song)
	if text, _ := lines.Current(112); text != "远处那盏灯亮着" {
		t.Errorf("lyrics at 1:52 = %q", text)
	}
}
