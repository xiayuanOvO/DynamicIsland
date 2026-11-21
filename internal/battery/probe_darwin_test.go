package battery

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

func TestMergeBluetoothProbe(t *testing.T) {
	connected := []Reading{
		{ID: "buds/0", Name: "测试耳机", Kind: Headphones, Percent: UnknownPercent},
		{ID: "mouse/0", Name: "测试鼠标", Kind: Mouse, Percent: UnknownPercent},
		{ID: "keyboard/0", Name: "测试键盘", Kind: Keyboard, Percent: 81},
		{ID: "duplicate-1/0", Name: "同名设备", Percent: UnknownPercent},
		{ID: "duplicate-2/0", Name: "同名设备", Percent: 67},
		{ID: "invalid/0", Name: "读数异常设备", Percent: UnknownPercent},
	}
	targets := bluetoothProbeTargets(connected)
	if want := (map[string]int{"测试耳机": 0, "测试鼠标": 1, "读数异常设备": 5}); !maps.Equal(targets, want) {
		t.Fatalf("got targets %v, want %v", targets, want)
	}
	output, err := json.Marshal([]struct {
		Name string
		Oppo bool
		Data []byte
	}{
		{Name: "测试耳机", Oppo: true, Data: []byte{0xaa, 15, 0, 0, 6, 0x81, 3, 8, 0, 0, 3, 2, 0xd1, 1, 73, 3, 0}},
		{Name: "测试鼠标", Data: []byte{38}},
		{Name: "测试键盘", Data: []byte{9}},
		{Name: "同名设备", Data: []byte{32}},
		{Name: "读数异常设备", Data: []byte{255}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Reading{
		{ID: "buds/1", Name: "测试耳机", Kind: Headphones, Part: Left, Percent: 73},
		{ID: "buds/2", Name: "测试耳机", Kind: Headphones, Part: Right, Percent: 81, Charging: true},
		{ID: "buds/3", Name: "测试耳机", Kind: Headphones, Part: Case, Percent: 0},
		{ID: "mouse/0", Name: "测试鼠标", Kind: Mouse, Percent: 38},
		connected[2], connected[3], connected[4], connected[5],
	}
	if got := mergeBluetoothProbe(connected, targets, output); !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestEncoBatteryPartsWithMissingOrInvalidComponents(t *testing.T) {
	device := Reading{ID: "buds/0", Name: "测试耳机", Kind: Headphones, Percent: UnknownPercent}
	reply := []byte{0xaa, 13, 0, 0, 6, 0x81, 3, 6, 0, 0, 2, 1, 62, 2, 0xff}
	want := []Reading{{ID: "buds/1", Name: "测试耳机", Kind: Headphones, Part: Left, Percent: 62}}
	if got := encoBatteryParts(device, reply); !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, bad := range [][]byte{reply[:10], reply[:14], slices.Concat(reply[:10], []byte{1}, reply[11:])} {
		if got := encoBatteryParts(device, bad); len(got) != 0 {
			t.Fatalf("malformed packet produced readings: %+v", got)
		}
	}
}
