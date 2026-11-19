package lyrics

import (
	"encoding/base64"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type lyricTransport func(*http.Request) (*http.Response, error)

func (f lyricTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLoadOnlineFallsBackToKugou(t *testing.T) {
	song := Song{Title: "月光列车 (女声版)", Artist: "林海风", Duration: 226}
	for _, test := range []struct {
		name, candidates string
		want             Lines
	}{
		{
			name:       "matching recording",
			candidates: `,{"id":"42","accesskey":"test-key","song":"月光列车 (女声版)","singer":"林海风","duration":226536}`,
			want:       Lines{{At: 12.5, Text: "夜里开来一辆列车"}},
		},
		{name: "only different recordings"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			client := &http.Client{Transport: lyricTransport(func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.Host)
				params := r.URL.Query()
				var body string
				switch r.URL.Host {
				case "lrclib.net":
					body = `[]`
				case "music.163.com":
					body = `{"code":200}`
				case "u.y.qq.com":
					body = `{"code":0,"req":{"code":0}}`
				case "mobiles.kugou.com":
					if r.URL.Path != "/api/v3/search/song" || params.Get("keyword") != song.Title+" "+song.Artist {
						t.Fatalf("unexpected search request: %s", r.URL)
					}
					body = `{"status":1,"data":{"info":[
						{"hash":"wrong-title","album_audio_id":1,"songname":"月光列车","singername":"林海风","duration":226},
						{"hash":"wrong-duration","album_audio_id":2,"songname":"月光列车 (女声版)","singername":"林海风","duration":245},
						{"hash":"matching-hash","album_audio_id":3,"songname":"月光列车 (女声版)","singername":"林海风","duration":226}
					]}}`
				case "krcs.kugou.com":
					if r.URL.Path != "/search" || params.Get("hash") != "matching-hash" || params.Get("album_audio_id") != "3" {
						t.Fatalf("unexpected candidate request: %s", r.URL)
					}
					body = `{"status":200,"candidates":[
						{"id":"40","accesskey":"test-key","song":"月光列车","singer":"林海风","duration":226000},
						{"id":"41","accesskey":"test-key","song":"月光列车 (女声版)","singer":"林海风","duration":245000}` + test.candidates + `]}`
				case "lyrics.kugou.com":
					if r.URL.Path != "/download" || params.Get("id") != "42" || params.Get("accesskey") != "test-key" || params.Get("fmt") != "lrc" {
						t.Fatalf("unexpected download request: %s", r.URL)
					}
					body = `{"status":200,"content":"` + base64.StdEncoding.EncodeToString([]byte("[00:12.50]夜里开来一辆列车")) + `"}`
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}

			if got := loadOnline(t.Context(), client, song); !slices.Equal(got, test.want) {
				t.Errorf("lyrics = %+v, want %+v", got, test.want)
			}
			wantCalls := []string{"lrclib.net", "music.163.com", "u.y.qq.com", "mobiles.kugou.com", "krcs.kugou.com"}
			if len(test.want) > 0 {
				wantCalls = append(wantCalls, "lyrics.kugou.com")
			}
			if !slices.Equal(calls, wantCalls) {
				t.Errorf("called %v, want %v", calls, wantCalls)
			}
		})
	}
}
