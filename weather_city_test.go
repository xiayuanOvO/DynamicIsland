package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSearchWeatherCitiesKeepsSameNameResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("name") != "上海" || q.Get("language") != "zh" {
			t.Errorf("unexpected search query %v", q)
		}
		fmt.Fprint(w, `{"results":[
			{"id":1,"name":"上海","admin1":"上海市","country":"中国","latitude":31.22222,"longitude":121.45806},
			{"id":2,"name":"上海","admin1":"浙江","country":"中国","latitude":29.32955,"longitude":121.05804},
			{"id":3,"name":"缺失坐标"}
		]}`)
	}))
	defer server.Close()

	cities, err := searchWeatherCities(context.Background(), server.URL, "上海")
	if err != nil || len(cities) != 2 {
		t.Fatalf("cities = %+v, error = %v", cities, err)
	}
	if cities[0].Region != "上海市 · 中国" || cities[1].Region != "浙江 · 中国" {
		t.Fatalf("same-name results require distinct regions: %+v", cities)
	}
	endpoint, err := url.Parse(cities[1].weatherEndpoint())
	if err != nil || endpoint.Path != "/~29.32955,121.05804" {
		t.Fatalf("forecast should use the selected location: %v, %v", endpoint, err)
	}
	if (weatherCity{}).weatherEndpoint() != idleWeatherEndpoint {
		t.Fatal("default city should use the automatic endpoint")
	}
}

func TestSettingsSelectAndResetWeatherCity(t *testing.T) {
	dir := t.TempDir()
	city := weatherCity{ID: 1, Name: "上海", Region: "上海市 · 中国", Latitude: 31.22222, Longitude: 121.45806}
	a := island{statePath: filepath.Join(dir, "shelf.json")}
	a.citySearch.query, a.citySearch.searched, a.citySearch.results = "上海", "上海", []weatherCity{city}
	a.idle.weather, a.idle.refreshAt = "晴 16°C", time.Now().Add(time.Hour)
	view := ui.NewTester(a.settingsView, settingsWidth, a.settingsHeight())
	for _, option := range []string{"自动定位", "上海 · 上海市 · 中国"} {
		if err := view.Click(option); err != nil {
			t.Fatal(err)
		}
	}
	view.Advance(time.Second)
	settings, err := loadSettings(a.settingsPath())
	if err != nil || settings.WeatherCity != city || a.idle.weather != "" || !a.idle.refreshAt.IsZero() {
		t.Fatalf("selected city should persist and invalidate the previous forecast: %+v, %v", settings, err)
	}
	for _, option := range []string{"上海", "恢复自动定位"} {
		if err := view.Click(option); err != nil {
			t.Fatal(err)
		}
	}
	view.Advance(time.Second)
	settings, err = loadSettings(a.settingsPath())
	if err != nil || settings.WeatherCity.ID != 0 {
		t.Fatalf("restored automatic location should persist: %+v, %v", settings, err)
	}
}
