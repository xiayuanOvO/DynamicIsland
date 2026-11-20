package nowplaying

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadNetEaseTimeline(t *testing.T) {
	sampled := time.Date(2026, 10, 22, 12, 30, 0, 0, time.Local)
	uptime := 30 * time.Minute
	track := Track{Title: "海岸", Artist: "林海风 / 李沐", Playing: true, Sampled: sampled}
	type event struct {
		secondsAgo int64
		pid        int
		tag, data  string
	}
	events := []event{
		{31, 42, "playOneTrackInPlayingList", `{"track":{"id":73,"name":"海岸","artists":[{"name":"林海风"},{"name":"李沐"}]}}`},
		{30, 42, "native播放资源load完成，开始播放", `{"songId":"73","songDuration":"183200"}`},
		{30, 42, "native播放state", "1,{}"},
		{25, 42, "setPlayingPosition", "65.25"},
		{20, 42, "native播放state", "2,{}"},
		{15, 42, "setPlayingPosition", "21.5"},
		{10, 42, "native播放state", "1,{}"},
		{6, 43, "setPlayingPosition", "95"},
	}
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	folder := filepath.Join(root, "NetEase", "CloudMusic")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		events []event
		paused bool
		skew   time.Duration
		want   float64
	}{
		{name: "playhead recovered when attaching mid-song", events: events[:3], want: 30},
		{name: "forward seek", events: events[:4], want: 90.25},
		{name: "paused playhead", events: events[:5], paused: true, want: 70.25},
		{name: "seek while paused", events: events[:6], paused: true, want: 21.5},
		{name: "resume after seek and ignore another process", events: events, want: 31.5},
		{name: "discard a mismatched playback state", events: events[:5]},
		{name: "discard records from a previous boot", events: events, skew: -24 * time.Hour},
		{
			name: "discard timeline when another song starts",
			events: append(events[:len(events):len(events)], event{
				2, 42, "native播放资源load完成，开始播放", `{"songId":"81","songDuration":"240000"}`,
			}),
		},
		{
			name: "restart same song on a new load",
			events: append(events[:len(events):len(events)],
				event{2, 42, "native播放资源load完成，开始播放", `{"songId":"73","songDuration":"183200"}`},
				event{2, 42, "native播放state", "1,{}"}),
			want: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var log strings.Builder
			log.WriteString("partial record\n")
			for _, line := range test.events {
				ago := time.Duration(line.secondsAgo) * time.Second
				fmt.Fprintf(&log, "[%d:9:1022/123000:%d:INFO:player(12)] [%s] 【playing】,%q,%s\n",
					line.pid, (uptime - ago).Milliseconds(), sampled.Add(test.skew-ago).Format("2006-01-02 15:04:05"), line.tag, line.data)
			}
			log.WriteString("【playing】,\"unterminated tail")
			if err := os.WriteFile(filepath.Join(folder, "cloudmusic.elog"), encodeNetEaseLog(log.String()), 0600); err != nil {
				t.Fatal(err)
			}
			track.Playing = !test.paused
			position, seconds := readNetEaseTimeline(track, uptime)
			var wantDuration float64
			if test.want > 0 {
				wantDuration = 183.2
			}
			if position != test.want || seconds != wantDuration {
				t.Errorf("got position %v and duration %v, want %v and %v", position, seconds, test.want, wantDuration)
			}
		})
	}
}
