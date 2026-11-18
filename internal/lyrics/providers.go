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
