package lyrics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
)

type creditedArtist struct {
	Name string
}

func artistNames(artists []creditedArtist) []string {
	names := make([]string, len(artists))
	for i, artist := range artists {
		names[i] = artist.Name
	}
	return names
}

func searchNetEase(ctx context.Context, client *http.Client, song Song) Lines {
	const host = "https://music.163.com"
	params := url.Values{"s": {song.Title + " " + song.Artist}, "type": {"1"}, "limit": {"10"}}
	var result struct {
		Code   int
		Result struct {
			Songs []struct {
				ID       int64
				Name     string
				Duration float64
				Album    struct{ Name string }
				Artists  []creditedArtist
			}
		}
	}
	if !fetchJSON(ctx, client, host+"/api/search/get/web?"+params.Encode(), host+"/", nil, &result) || result.Code != 200 {
		return nil
	}

	var id int64
	best := math.Inf(1)
	for _, candidate := range result.Result.Songs {
		if score := matchScore(song, candidate.Name, artistNames(candidate.Artists), candidate.Album.Name, candidate.Duration/1000); candidate.ID > 0 && score < best {
			id, best = candidate.ID, score
		}
	}
	if id == 0 {
		return nil
	}

	params = url.Values{"id": {strconv.FormatInt(id, 10)}, "lv": {"1"}, "kv": {"1"}, "tv": {"-1"}}
	var lyrics struct {
		Code int
		LRC  struct{ Lyric string }
	}
	if !fetchJSON(ctx, client, host+"/api/song/lyric?"+params.Encode(), host+"/", nil, &lyrics) || lyrics.Code != 200 {
		return nil
	}
	return Parse(lyrics.LRC.Lyric)
}

func searchQQ(ctx context.Context, client *http.Client, song Song) Lines {
	const referer = "https://y.qq.com/"
	query, err := json.Marshal(map[string]any{
		"req": map[string]any{
			"module": "music.search.SearchCgiService",
			"method": "DoSearchForQQMusicDesktop",
			"param": map[string]any{
				"query": song.Title + " " + song.Artist, "search_type": 0,
				"page_num": 1, "num_per_page": 10,
			},
		},
	})
	if err != nil {
		return nil
	}
	var result struct {
		Code int
		Req  struct {
			Code int
			Data struct {
				Body struct {
					Song struct {
						List []struct {
							Mid, Name string
							Interval  float64
							Album     struct{ Name string }
							Singer    []creditedArtist
						}
					}
				}
			}
		}
	}
	if !fetchJSON(ctx, client, "https://u.y.qq.com/cgi-bin/musicu.fcg", referer, query, &result) || result.Code != 0 || result.Req.Code != 0 {
		return nil
	}

	var id string
	best := math.Inf(1)
	for _, candidate := range result.Req.Data.Body.Song.List {
		if score := matchScore(song, candidate.Name, artistNames(candidate.Singer), candidate.Album.Name, candidate.Interval); candidate.Mid != "" && score < best {
			id, best = candidate.Mid, score
		}
	}
	if id == "" {
		return nil
	}
	params := url.Values{"songmid": {id}, "format": {"json"}, "nobase64": {"0"}}
	var lyrics struct {
		Code  int
		Lyric string
	}
	if !fetchJSON(ctx, client, "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg?"+params.Encode(), referer, nil, &lyrics) || lyrics.Code != 0 {
		return nil
	}
	text, err := base64.StdEncoding.DecodeString(lyrics.Lyric)
	if err != nil {
		return nil
	}
	return Parse(html.UnescapeString(string(text)))
}

func searchKugou(ctx context.Context, client *http.Client, song Song) Lines {
	params := url.Values{
		"keyword": {song.Title + " " + song.Artist}, "format": {"json"},
		"page": {"1"}, "pagesize": {"10"}, "showtype": {"1"},
	}
	var result struct {
		Status int
		Data   struct {
			Info []struct {
				Hash, SongName, SingerName string
				AlbumName                  string `json:"album_name"`
				AlbumAudioID               int64  `json:"album_audio_id"`
				Duration                   float64
			}
		}
	}
	if !fetchJSON(ctx, client, "https://mobiles.kugou.com/api/v3/search/song?"+params.Encode(), "", nil, &result) || result.Status != 1 {
		return nil
	}

	var hash string
	var albumAudioID int64
	best := math.Inf(1)
	for _, track := range result.Data.Info {
		if score := matchScore(song, track.SongName, []string{track.SingerName}, track.AlbumName, track.Duration); track.Hash != "" && score < best {
			hash, albumAudioID, best = track.Hash, track.AlbumAudioID, score
		}
	}
	if hash == "" {
		return nil
	}

	params = url.Values{"ver": {"1"}, "man": {"yes"}, "client": {"mobi"}, "hash": {hash}}
	if albumAudioID > 0 {
		params.Set("album_audio_id", strconv.FormatInt(albumAudioID, 10))
	}
	var matches struct {
		Status     int
		Candidates []struct {
			ID, AccessKey, Song, Singer string
			Duration                    float64
		}
	}
	if !fetchJSON(ctx, client, "https://krcs.kugou.com/search?"+params.Encode(), "", nil, &matches) || matches.Status != 200 {
		return nil
	}

	var id, accessKey string
	best = math.Inf(1)
	for _, candidate := range matches.Candidates {
		if score := matchScore(song, candidate.Song, []string{candidate.Singer}, "", candidate.Duration/1000); candidate.ID != "" && candidate.AccessKey != "" && score < best {
			id, accessKey, best = candidate.ID, candidate.AccessKey, score
		}
	}
	if id == "" {
		return nil
	}

	params = url.Values{
		"id": {id}, "accesskey": {accessKey}, "fmt": {"lrc"},
		"charset": {"utf8"}, "client": {"pc"}, "ver": {"1"},
	}
	var lyrics struct {
		Status  int
		Content string
	}
	if !fetchJSON(ctx, client, "https://lyrics.kugou.com/download?"+params.Encode(), "", nil, &lyrics) || lyrics.Status != 200 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(lyrics.Content)
	if err != nil {
		return nil
	}
	return Parse(string(data))
}
