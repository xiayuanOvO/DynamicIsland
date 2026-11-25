package island

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	weatherSourceOpenMeteo = iota
	weatherSourceWttr
)

const (
	openMeteoForecastEndpoint = "https://api.open-meteo.com/v1/forecast"
	ipGeoEndpoint             = "https://get.geojs.io/v1/ip/geo.json"
	weatherRefreshInterval    = time.Hour
	weatherFetchTimeout       = 15 * time.Second
)

type weatherCacheDay struct {
	Date        string `json:"date"`
	Code        int    `json:"code"`
	Condition   string `json:"condition"`
	Temperature int    `json:"temperature"`
	MinTemp     int    `json:"minTemp,omitempty"`
	HasMin      bool   `json:"hasMin,omitempty"`
	Sunrise     string `json:"sunrise,omitempty"`
	Sunset      string `json:"sunset,omitempty"`
	Rain        bool   `json:"rain"`
}

type weatherCacheHour struct {
	Time        string `json:"time"`
	Code        int    `json:"code"`
	Temperature int    `json:"temperature"`
}

type weatherCache struct {
	CityKey   string             `json:"cityKey"`
	CityName  string             `json:"cityName,omitempty"`
	Source    string             `json:"source"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Current   weatherCacheDay    `json:"current"`
	Days      []weatherCacheDay  `json:"days,omitempty"`
	Hourly    []weatherCacheHour `json:"hourly,omitempty"`
}

func (c weatherCity) cacheKey() string {
	if c.ID == 0 {
		return "auto"
	}
	return fmt.Sprintf("%d:%.5f,%.5f", c.ID, c.Latitude, c.Longitude)
}

func (a *island) weatherCachePath() string {
	return filepath.Join(filepath.Dir(a.statePath), "weather-cache.json")
}

func loadWeatherCache(path string) (weatherCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return weatherCache{}, err
	}
	var cache weatherCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return weatherCache{}, err
	}
	return cache, nil
}

func saveWeatherCache(path string, cache weatherCache) error {
	return saveJSON(path, cache)
}

func weatherDataEqual(a, b weatherCache) bool {
	return a.CityKey == b.CityKey && a.CityName == b.CityName && a.Current == b.Current &&
		slices.Equal(a.Days, b.Days) && slices.Equal(a.Hourly, b.Hourly)
}

func weatherFromCache(cache weatherCache, now time.Time) (idleWeather, time.Time, bool) {
	var rainDates []string
	for _, day := range cache.Days {
		if day.Rain {
			rainDates = append(rainDates, day.Date)
		}
	}
	forecastUntil := now.Add(24 * time.Hour)
	if len(cache.Days) > 0 {
		if last, err := time.ParseInLocation(time.DateOnly, cache.Days[len(cache.Days)-1].Date, now.Location()); err == nil {
			forecastUntil = last.Add(24 * time.Hour)
		}
	}
	if cache.Current.Condition != "" && now.Sub(cache.UpdatedAt) < 2*time.Hour {
		return idleWeather{
			condition:   cache.Current.Condition,
			temperature: cache.Current.Temperature,
			icon:        weatherIconForSource(cache.Source, cache.Current.Code, cache.Current.Condition),
			rainDates:   rainDates,
		}, forecastUntil, true
	}
	today := now.Format(time.DateOnly)
	for _, day := range cache.Days {
		if day.Date == today && day.Condition != "" {
			return idleWeather{
				condition:   day.Condition,
				temperature: day.Temperature,
				icon:        weatherIconForSource(cache.Source, day.Code, day.Condition),
				rainDates:   rainDates,
			}, forecastUntil, true
		}
	}
	return idleWeather{}, time.Time{}, false
}

func weatherIconForSource(source string, code int, condition string) *ui.SVG {
	if source == "open-meteo" {
		return wmoWeatherIcon(code)
	}
	return idleWeatherIcon(code, condition)
}

func wmoWeatherInfo(code int) (condition string, icon *ui.SVG, rain bool) {
	switch code {
	case 0:
		return "晴", weatherSunIcon, false
	case 1, 2:
		return "多云", weatherPartlyCloudyIcon, false
	case 3:
		return "阴", weatherCloudIcon, false
	case 45, 48:
		return "雾", weatherFogIcon, false
	case 51, 53, 55, 56, 57:
		return "毛毛雨", weatherRainIcon, true
	case 61, 63, 65, 66, 67, 80, 81, 82:
		return "雨", weatherRainIcon, true
	case 71, 73, 75, 77, 85, 86:
		return "雪", weatherSnowIcon, false
	case 95, 96, 99:
		return "雷阵雨", weatherThunderIcon, true
	default:
		return "未知天气", weatherUnknownIcon, false
	}
}

func wmoWeatherIcon(code int) *ui.SVG {
	_, icon, _ := wmoWeatherInfo(code)
	return icon
}

func fetchIPCoordinates(ctx context.Context, endpoint string) (float64, float64, error) {
	lat, lon, _, err := fetchIPLocation(ctx, endpoint)
	return lat, lon, err
}

func fetchIPLocation(ctx context.Context, endpoint string) (float64, float64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, 0, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("ip geo HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Latitude  json.Number `json:"latitude"`
		Longitude json.Number `json:"longitude"`
		City      string      `json:"city"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&payload); err != nil {
		return 0, 0, "", err
	}
	lat, err := payload.Latitude.Float64()
	if err != nil {
		return 0, 0, "", err
	}
	lon, err := payload.Longitude.Float64()
	if err != nil {
		return 0, 0, "", err
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, "", errors.New("invalid ip coordinates")
	}
	return lat, lon, payload.City, nil
}

func fetchOpenMeteoWeather(ctx context.Context, endpoint string, lat, lon float64, cityKey string) (weatherCache, error) {
	url := fmt.Sprintf("%s?latitude=%.5f&longitude=%.5f&current=temperature_2m,weather_code&hourly=temperature_2m,weather_code&daily=weather_code,temperature_2m_max,temperature_2m_min,sunrise,sunset&timezone=auto&forecast_days=14&forecast_hours=24",
		endpoint, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return weatherCache{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return weatherCache{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return weatherCache{}, fmt.Errorf("open-meteo HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Current struct {
			Temperature *float64 `json:"temperature_2m"`
			Code        *int     `json:"weather_code"`
		} `json:"current"`
		Hourly struct {
			Time        []string  `json:"time"`
			Code        []int     `json:"weather_code"`
			Temperature []float64 `json:"temperature_2m"`
		} `json:"hourly"`
		Daily struct {
			Time           []string  `json:"time"`
			Code           []int     `json:"weather_code"`
			Temperature    []float64 `json:"temperature_2m_max"`
			TemperatureMin []float64 `json:"temperature_2m_min"`
			Sunrise        []string  `json:"sunrise"`
			Sunset         []string  `json:"sunset"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&payload); err != nil {
		return weatherCache{}, err
	}
	if payload.Current.Temperature == nil || payload.Current.Code == nil {
		return weatherCache{}, errors.New("invalid open-meteo response")
	}
	temp := int(math.Round(*payload.Current.Temperature))
	if temp < -100 || temp > 80 {
		return weatherCache{}, errors.New("invalid open-meteo temperature")
	}
	cond, _, rain := wmoWeatherInfo(*payload.Current.Code)
	cache := weatherCache{
		CityKey:   cityKey,
		Source:    "open-meteo",
		UpdatedAt: time.Now(),
		Current: weatherCacheDay{
			Date:        time.Now().Format(time.DateOnly),
			Code:        *payload.Current.Code,
			Condition:   cond,
			Temperature: temp,
			Rain:        rain,
		},
	}
	for i := 0; i < len(payload.Daily.Time) && i < len(payload.Daily.Code); i++ {
		dCond, _, dRain := wmoWeatherInfo(payload.Daily.Code[i])
		dTemp := temp
		if i < len(payload.Daily.Temperature) {
			dTemp = int(math.Round(payload.Daily.Temperature[i]))
		}
		day := weatherCacheDay{
			Date:        payload.Daily.Time[i],
			Code:        payload.Daily.Code[i],
			Condition:   dCond,
			Temperature: dTemp,
			Rain:        dRain,
		}
		if i < len(payload.Daily.TemperatureMin) {
			day.MinTemp = int(math.Round(payload.Daily.TemperatureMin[i]))
			day.HasMin = true
		}
		if i < len(payload.Daily.Sunrise) {
			day.Sunrise = payload.Daily.Sunrise[i]
		}
		if i < len(payload.Daily.Sunset) {
			day.Sunset = payload.Daily.Sunset[i]
		}
		cache.Days = append(cache.Days, day)
	}
	for i := 0; i < len(payload.Hourly.Time) && i < len(payload.Hourly.Code) && i < len(payload.Hourly.Temperature); i++ {
		cache.Hourly = append(cache.Hourly, weatherCacheHour{
			Time:        payload.Hourly.Time[i],
			Code:        payload.Hourly.Code[i],
			Temperature: int(math.Round(payload.Hourly.Temperature[i])),
		})
	}
	return cache, nil
}

func fetchWeatherWithFallback(ctx context.Context, primary int, city weatherCity) (weatherCache, error) {
	order := []int{weatherSourceOpenMeteo, weatherSourceWttr}
	if primary == weatherSourceWttr {
		order = []int{weatherSourceWttr, weatherSourceOpenMeteo}
	}
	var lastErr error
	for _, src := range order {
		switch src {
		case weatherSourceOpenMeteo:
			lat, lon := city.Latitude, city.Longitude
			cityName := city.Name
			if city.ID == 0 {
				var err error
				lat, lon, cityName, err = fetchIPLocation(ctx, ipGeoEndpoint)
				if err != nil {
					log.Printf("weather open-meteo ip geo failed: %v", err)
					lastErr = err
					continue
				}
			}
			cache, err := fetchOpenMeteoWeather(ctx, openMeteoForecastEndpoint, lat, lon, city.cacheKey())
			if err == nil {
				if cache.CityName == "" {
					cache.CityName = cityName
				}
				return cache, nil
			}
			log.Printf("weather open-meteo failed: %v", err)
			lastErr = err
		case weatherSourceWttr:
			cache, err := fetchWttrWeatherCache(ctx, city.weatherEndpoint(), city.cacheKey())
			if err == nil {
				if city.Name != "" {
					cache.CityName = city.Name
				}
				return cache, nil
			}
			log.Printf("weather wttr.in failed: %v", err)
			lastErr = err
		}
	}
	return weatherCache{}, lastErr
}
