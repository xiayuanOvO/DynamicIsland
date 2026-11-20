package nowplaying

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const netEaseLogTailSize = 2 << 20

var netEaseLogHeader = regexp.MustCompile(`^\[(\d+):\d+:\d{4}/\d{6}:(\d+):[^]]+\]\s+\[([^]]+)\]`)

// The byte encoding and playback event names are also documented by
// https://github.com/Coooookies/netease-cloudmusic-detector.
var (
	netEaseLogPrefix  = encodeNetEaseLog("【playing】,\"")
	netEaseLogPayload = encodeNetEaseLog("\",")
	netEaseLogNewline = encodeNetEaseLog("\n")
	netEaseLogEvents  = map[string]string{
		string(encodeNetEaseLog("checkPlayPrivilege")):        "checkPlayPrivilege",
		string(encodeNetEaseLog("playOneTrackInPlayingList")): "playOneTrackInPlayingList",
		string(encodeNetEaseLog("native播放资源load完成，开始播放")):     "native播放资源load完成，开始播放",
		string(encodeNetEaseLog("setPlayingPosition")):        "setPlayingPosition",
		string(encodeNetEaseLog("native播放state")):             "native播放state",
	}
)

func encodeNetEaseLog(text string) []byte {
	encoded := []byte(text)
	for i, v := range encoded {
		high := (v & 15) ^ 3
		encoded[i] = high<<4 | (v>>4 ^ high ^ 8)
	}
	return encoded
}

func readNetEaseTimeline(track Track, uptime time.Duration) (position, duration float64) {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return 0, 0
	}
	file, err := os.Open(filepath.Join(root, "NetEase", "CloudMusic", "cloudmusic.elog"))
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, 0
	}
	data := make([]byte, min(info.Size(), netEaseLogTailSize))
	if _, err := file.ReadAt(data, info.Size()-int64(len(data))); err != nil {
		return 0, 0
	}
	return netEaseTimeline(data, track, uptime)
}

type netEaseLogSong struct {
	ID      json.Number
	Name    string
	Artists []struct{ Name string }
}

func netEaseTimeline(data []byte, track Track, uptime time.Duration) (position, duration float64) {
	var clock struct {
		pid              string
		tick             int64
		position, length float64
		started, playing bool
	}
	matched := make(map[json.Number]string)
	nowTick := uptime.Milliseconds()
	title, artist := strings.TrimSpace(track.Title), strings.TrimSpace(track.Artist)
	for line := range bytes.SplitSeq(data, netEaseLogNewline) {
		start := bytes.Index(line, netEaseLogPrefix)
		if start == -1 {
			continue
		}
		kind, payload, ok := bytes.Cut(line[start+len(netEaseLogPrefix):], netEaseLogPayload)
		tag := netEaseLogEvents[string(kind)]
		if !ok || tag == "" {
			continue
		}
		for i, v := range line {
			high := v >> 4
			line[i] = (high^(v&15)^8)<<4 | (high ^ 3)
		}
		header := netEaseLogHeader.FindSubmatch(line[:start])
		if header == nil {
			continue
		}
		tick, err := strconv.ParseInt(string(header[2]), 10, 64)
		if err != nil || tick > nowTick {
			continue
		}
		wall, err := time.ParseInLocation("2006-01-02 15:04:05", string(header[3]), track.Sampled.Location())
		// The elapsed counter restarts when Windows boots. Check the recorded date as well.
		if skew := track.Sampled.Sub(wall) - time.Duration(nowTick-tick)*time.Millisecond; err != nil || skew < -2*time.Second || skew > 3*time.Second {
			continue
		}
		pid := string(header[1])
		// Each event begins with one JSON value and can have additional fields afterward.
		decoder := json.NewDecoder(bytes.NewReader(payload))
		switch tag {
		case "checkPlayPrivilege", "playOneTrackInPlayingList":
			var event struct {
				netEaseLogSong
				Track netEaseLogSong
			}
			if decoder.Decode(&event) != nil {
				continue
			}
			song := event.netEaseLogSong
			if tag == "playOneTrackInPlayingList" {
				song = event.Track
			}
			if song.ID == "" || !strings.EqualFold(strings.TrimSpace(song.Name), title) {
				continue
			}
			names := make([]string, 0, len(song.Artists))
			for _, credit := range song.Artists {
				name := strings.TrimSpace(credit.Name)
				names = append(names, name)
				if strings.EqualFold(artist, name) {
					matched[song.ID] = pid
				}
			}
			if strings.EqualFold(artist, strings.Join(names, "/")) || strings.EqualFold(artist, strings.Join(names, " / ")) {
				matched[song.ID] = pid
			}
		case "native播放资源load完成，开始播放":
			clock.pid, clock.started, clock.playing = "", false, false
			var event struct {
				SongID, SongDuration json.Number
			}
			if decoder.Decode(&event) != nil || matched[event.SongID] != pid {
				continue
			}
			length, err := event.SongDuration.Float64()
			if err == nil && length > 0 {
				clock.pid, clock.tick, clock.position, clock.length = pid, tick, 0, length/1000
			}
		case "native播放state":
			if pid != clock.pid || tick < clock.tick {
				continue
			}
			var status int
			if decoder.Decode(&status) != nil || (status != 1 && status != 2) {
				continue
			}
			if clock.playing {
				clock.position += float64(tick-clock.tick) / 1000
			}
			clock.tick, clock.started, clock.playing = tick, true, status == 1
		case "setPlayingPosition":
			if pid != clock.pid || tick < clock.tick {
				continue
			}
			var seek float64
			if decoder.Decode(&seek) == nil && seek >= 0 && seek <= clock.length {
				clock.tick, clock.position = tick, seek
			}
		}
	}
	if !clock.started || clock.playing != track.Playing {
		return 0, 0
	}
	position = clock.position
	if clock.playing {
		position += float64(nowTick-clock.tick) / 1000
	}
	if position > clock.length+1 {
		return 0, 0
	}
	return min(position, clock.length), clock.length
}
