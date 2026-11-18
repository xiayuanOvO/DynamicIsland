package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dynamicisland/internal/nowplaying"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestIdleInfoMatchesBallWhenMediaIsPaused(t *testing.T) {
	a := island{files: &shelf.Shelf{}}
	a.media.track = nowplaying.Track{Title: "暂停的曲目"}
	if !a.compactIdle() || !a.showsIdleInfo() {
		t.Fatal("an idle ball should show time and weather even when a paused session is retained")
	}
	a.expanded = true
	if a.showsIdleInfo() {
		t.Fatal("idle info must hide while the media card is expanded")
	}
	a.expanded = false
	a.media.track.Playing = true
	if a.showsIdleInfo() {
		t.Fatal("idle info must hide during playback")
	}
}

func TestIdleClockKeepsTimeWithAndWithoutWeather(t *testing.T) {
	info := idleInfo{weather: "多云 18°C", weatherUntil: time.Now().Add(time.Hour)}
	view := ui.NewTester(info.view, idleInfoWidth, compactHeight)
	if _, found := view.Find("多云 18°C"); !found {
		t.Fatal("weather missing")
	}
	view.Advance(time.Minute)
	if _, found := view.Find(view.Now().Format("15:04")); !found {
		t.Fatal("clock did not advance")
	}
	view.Advance(time.Hour)
	if _, found := view.Find("多云 18°C"); found {
		t.Fatal("expired weather was displayed")
	}
	if _, found := view.Find(view.Now().Format("15:04")); !found {
		t.Fatal("clock disappeared with weather")
	}
}

func TestIdleInfoStaysBesideBallInsideDisplay(t *testing.T) {
	area := mygo.Rectangle{X: -800, Y: 30, Width: 800, Height: 500}
	ball := mygo.Rectangle{X: -410, Y: 40, Width: compactHeight, Height: compactHeight}
	bounds, align := idleInfoBounds(ball, area)
	if align != ui.Start || bounds.X <= ball.X+ball.Width || bounds.Y != ball.Y {
		t.Fatalf("expected info to the right: %+v, %v", bounds, align)
	}
	ball.X = -compactHeight - 10 - 120
	bounds, align = idleInfoBounds(ball, area)
	if align != ui.Start || bounds.Width != 120 {
		t.Fatalf("expected to preserve right alignment when there is room: %+v, %v", bounds, align)
	}
	ball.X = -compactHeight / 2
	bounds, align = idleInfoBounds(ball, area)
	if align != ui.End || bounds.X+bounds.Width >= ball.X || bounds.X < area.X {
		t.Fatalf("expected info to the left of the right-docked ball: %+v, %v", bounds, align)
	}
}

func TestFetchIdleWeather(t *testing.T) {
	for _, test := range []struct {
		response, want string
		status         int
	}{
		{"晴|+17°C\n", "晴 17°C", http.StatusOK},
		{"小雪|-5°C", "小雪 -5°C", http.StatusOK},
		{"no forecast", "", http.StatusOK},
		{"晴|+17°C", "", http.StatusServiceUnavailable},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(test.status)
			fmt.Fprint(w, test.response)
		}))
		actual, err := fetchIdleWeather(context.Background(), server.URL)
		server.Close()
		if actual != test.want || (err == nil) != (test.want != "") {
			t.Fatalf("response %q produced %q, %v; want %q", test.response, actual, err, test.want)
		}
	}
}
