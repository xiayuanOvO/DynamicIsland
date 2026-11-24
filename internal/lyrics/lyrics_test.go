package lyrics

import (
	"slices"
	"testing"
)

func TestSimplifiedLyrics(t *testing.T) {
	original := Lines{
		{At: 1.5, Text: "風把頭髮吹亂了，Hello! 🎵"},
		{At: 3, Text: "乾隆年間的乾燥花"},
		{At: 4, Text: "窗外亮起一盏灯"},
		{At: 5, Text: "約束を忘れない"},
		{At: 6},
	}
	source := slices.Clone(original)
	want := slices.Clone(original)
	want[0].Text = "风把头发吹乱了，Hello! 🎵"
	want[1].Text = "乾隆年间的干燥花"
	if got := source.Simplified(); !slices.Equal(got, want) {
		t.Fatalf("Simplified() = %v, want %v", got, want)
	}
	if !slices.Equal(source, original) {
		t.Fatalf("Simplified() changed the source lyrics: %v", source)
	}
}

func TestLyricsMatchSongLanguage(t *testing.T) {
	englishOnly := Lines{{At: 19.62, Text: "There are always some words too late to say"}}
	chinese := Lines{{At: 19.62, Text: "总有些话来不及说"}}

	song := Song{Title: "如果爱忘了", Artist: "汪苏泷"}
	if lyricsMatchSongLanguage(song, englishOnly) {
		t.Fatal("Chinese-titled song should not accept pure-English translated lyrics when Chinese sources exist")
	}
	if !lyricsMatchSongLanguage(song, chinese) {
		t.Fatal("Chinese-titled song should accept Chinese lyrics")
	}
	englishVer := Song{Title: "如果爱忘了 (English Version)", Artist: "汪苏泷"}
	if !lyricsMatchSongLanguage(englishVer, englishOnly) {
		t.Fatal("explicit English version should allow English lyrics")
	}
	foreign := Song{Title: "Shape of You", Artist: "Ed Sheeran"}
	if !lyricsMatchSongLanguage(foreign, englishOnly) {
		t.Fatal("non-Chinese song should accept English lyrics")
	}
}
