package lyrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTimedLines(t *testing.T) {
	lines := Parse("[00:06.40][00:10.00]再见\n[00:01.000]你好\n[00:12.00]\n[offset:100]\n[ar:歌手]")
	for _, want := range []struct {
		at   float64
		text string
		next float64
	}{
		{0, "", 1.1},
		{2, "你好", 6.5},
		{7, "再见", 10.1},
		{11, "再见", 12.1},
		{13, "", 0},
		{2, "你好", 6.5},
	} {
		text, next := lines.Current(want.at)
		if text != want.text || next != want.next {
			t.Errorf("at %.2f got %q, %.2f; want %q, %.2f", want.at, text, next, want.text, want.next)
		}
	}
}

func TestSurroundingLinesFollowPlayback(t *testing.T) {
	lines := Parse("[00:01]第一句\n[00:02]\n[00:05]第二句\n[00:08]第三句")
	for _, test := range []struct {
		position float64
		rows     [3]string
		next     float64
	}{
		{1, [3]string{"", "第一句", "第二句"}, 2},
		{3, [3]string{"第一句", "", "第二句"}, 5},
		{6, [3]string{"第一句", "第二句", "第三句"}, 8},
		{9, [3]string{"第二句", "第三句", ""}, 0},
		{1, [3]string{"", "第一句", "第二句"}, 2},
	} {
		rows, next := lines.Surrounding(test.position)
		if rows != test.rows || next != test.next {
			t.Fatalf("at %v: got %q, %v, want %q, %v", test.position, rows, next, test.rows, test.next)
		}
	}
}

func TestLocalLyricsTakePriority(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "歌手 - 歌名.lrc"), []byte("[00:00.00]歌词"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lines := Load(ctx, dir, Song{Title: "歌名", Artist: "歌手"})
	if line, _ := lines.Current(0); line != "歌词" {
		t.Fatalf("did not load local lyrics: %q", line)
	}
}

func TestSearchChoosesMatchingVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("track_name") != "歌名" || query.Get("artist_name") != "歌手" {
			t.Errorf("unexpected query: %v", query)
		}
		io.WriteString(w, `[
			{"trackName":"歌名","artistName":"其他歌手","duration":140,"syncedLyrics":"[00:00.00]wrong artist"},
			{"trackName":"歌名","artistName":"歌手","duration":160,"syncedLyrics":"[00:00.00]wrong version"},
			{"trackName":"歌名","artistName":"歌手","duration":141,"syncedLyrics":"[00:00.00]correct"}
		]`)
	}))
	defer server.Close()

	lines := search(context.Background(), server.Client(), server.URL, Song{Title: "歌名", Artist: "歌手", Duration: 140})
	if line, _ := lines.Current(0); line != "correct" {
		t.Fatalf("wrong lyric match: %q", line)
	}
}

type lyricTransport func(*http.Request) (*http.Response, error)

func (f lyricTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOnlineLyricsFallback(t *testing.T) {
	for _, test := range []struct {
		name, lrclib, netEase, want string
		wantHosts                   []string
	}{
		{"LRCLIB", `[{"trackName":"歌名","artistName":"别名","duration":254,"syncedLyrics":"[00:00.00]首选"}]`, "", "首选", []string{"lrclib.net"}},
		{"NetEase alias", "[]", `{"code":200,"lrc":{"lyric":"[00:00.00]网易云"}}`, "网易云", []string{"lrclib.net", "music.163.com", "music.163.com"}},
		{"QQ fallback", "[]", `{"code":200,"lrc":{"lyric":"[00:00.00]"}}`, "QQ", []string{"lrclib.net", "music.163.com", "music.163.com", "u.y.qq.com", "c.y.qq.com"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var hosts []string
			client := &http.Client{Transport: lyricTransport(func(r *http.Request) (*http.Response, error) {
				hosts = append(hosts, r.URL.Host)
				var body string
				switch r.URL.Path {
				case "/api/search":
					body = test.lrclib
				case "/api/search/get/web":
					body = `{"code":200,"result":{"songs":[
						{"id":1,"name":"歌名","duration":254000,"album":{"name":"其他专辑"},"artists":[{"name":"其他艺人"}]},
						{"id":2,"name":"歌名","duration":254000,"album":{"name":"专辑"},"artists":[{"name":"本名"}]}
					]}}`
				case "/api/song/lyric":
					if r.URL.Query().Get("id") != "2" {
						t.Errorf("selected a same-title track from the wrong album: %s", r.URL)
					}
					body = test.netEase
				case "/cgi-bin/musicu.fcg":
					body = `{"code":0,"req":{"code":0,"data":{"body":{"song":{"list":[
						{"mid":"QQ1","name":"歌名","interval":254,"album":{"name":"专辑"},"singer":[{"name":"本名"}]}
					]}}}}}`
				case "/lyric/fcgi-bin/fcg_query_lyric_new.fcg":
					if r.URL.Query().Get("songmid") != "QQ1" {
						t.Errorf("did not request the matching QQ version: %s", r.URL)
					}
					body = `{"code":0,"lyric":"WzAwOjAwLjAwXVFR"}`
				default:
					t.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			lines := loadOnline(context.Background(), client, Song{Title: "歌名", Artist: "别名", Album: "专辑", Duration: 254})
			if got, _ := lines.Current(0); got != test.want || !slices.Equal(hosts, test.wantHosts) {
				t.Fatalf("got %q from %v, want %q from %v", got, hosts, test.want, test.wantHosts)
			}
		})
	}
}
