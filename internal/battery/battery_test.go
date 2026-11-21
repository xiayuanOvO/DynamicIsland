package battery

import (
	"slices"
	"testing"
)

func TestSnapshotMergeRetainsUnreadSources(t *testing.T) {
	computer := Reading{ID: "computer", Name: "本机", Kind: Computer, Percent: 80}
	mouse := Reading{ID: "mouse", Name: "测试鼠标", Kind: Mouse, Percent: 64}
	cached := []Reading{computer, mouse}
	newComputer, newMouse := computer, mouse
	newComputer.Percent, newMouse.Percent = 79, 63

	for _, test := range []struct {
		name     string
		snapshot Snapshot
		want     []Reading
	}{
		{"no usable results", Snapshot{}, cached},
		{"bluetooth probe failed", Snapshot{computer: []Reading{newComputer}}, []Reading{newComputer, mouse}},
		{"computer probe failed", Snapshot{bluetooth: []Reading{newMouse}}, []Reading{computer, newMouse}},
		{"devices disconnected", Snapshot{bluetooth: []Reading{}}, []Reading{computer}},
		{"computer battery absent", Snapshot{computer: []Reading{}}, []Reading{mouse}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.snapshot.Merge(cached); !slices.Equal(got, test.want) {
				t.Fatalf("merged readings = %+v, want %+v", got, test.want)
			}
		})
	}
}
