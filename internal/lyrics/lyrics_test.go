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
