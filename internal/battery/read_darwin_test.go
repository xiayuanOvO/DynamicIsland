package battery

import (
	"slices"
	"testing"
)

func TestParseMacBattery(t *testing.T) {
	for _, test := range []struct {
		name, report string
		present      bool
		percent      int
		charging     bool
	}{
		{"charging", "Now drawing from 'AC Power'\n -InternalBattery-0 (id=123)\t85%; charging; 1:20 remaining present: true\n", true, 85, true},
		{"charged", " -InternalBattery-0 (id=123)\t100%; charged; 0:00 remaining present: true\n", true, 100, false},
		{"not charging", " -InternalBattery-0 (id=123)\t55%; AC attached; not charging present: true\n", true, 55, false},
		{"empty", " -InternalBattery-0 (id=123)\t0%; discharging; (no estimate) present: true\n", true, 0, false},
		{"absent", " -InternalBattery-0 (id=123)\t0%; discharging present: false\n", false, 0, false},
		{"external only", " -UPS-0 (id=123)\t100%; charged present: true\n", false, 0, false},
		{"unknown", " -InternalBattery-0 (id=123)\t255%; unknown\n", false, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, ok := parseMacBattery(test.report)
			if ok != test.present || r.Percent != test.percent || r.Charging != test.charging {
				t.Fatalf("got (%+v, %v)", r, ok)
			}
		})
	}
}

func TestBluetoothBatteries(t *testing.T) {
	ioreg := registryBatteries([]byte(`<plist><array>
		<dict><key>DeviceAddress</key><string>00-00-00-00-00-A1</string><key>Nested</key><dict><key>BatteryPercent</key><integer>9</integer></dict><key>BatteryPercent</key><integer>68</integer></dict>
		<dict><key>DeviceAddress</key><string>00-00-00-00-00-A2</string><key>BatteryPercent</key><integer>255</integer></dict>
	</array></plist>`))
	output := []byte(`{"SPBluetoothDataType":[{"device_connected":[
		{"Mouse":{"device_address":"00:00:00:00:00:a1","device_minorType":"Mouse"}},
		{"Earbuds":{"device_address":"00:00:00:00:00:a0","device_minorType":"Headset","device_batteryLevelMain":"82%","device_batteryLevelLeft":"75%","device_batteryLevelRight":62,"device_batteryLevelCase":"0%"}},
		{"Headset":{"device_address":"00:00:00:00:00:a2","device_minorType":"Headphones","device_batteryLevelMain":"Unknown"}},
		{"Case only":{"device_address":"00:00:00:00:00:a5","device_batteryLevelCase":"95%"}},
		{"Mouse":{"device_address":"00:00:00:00:00:a1","device_minorType":"Mouse"}}
	],"device_not_connected":[{"Offline":{"device_address":"00:00:00:00:00:a3","device_batteryLevelMain":"100%"}}]}]}`)
	want := []Reading{
		{ID: "00:00:00:00:00:a5/3", Name: "Case only", Part: Case, Percent: 95},
		{ID: "00:00:00:00:00:a0/1", Name: "Earbuds", Kind: Headphones, Part: Left, Percent: 75},
		{ID: "00:00:00:00:00:a0/2", Name: "Earbuds", Kind: Headphones, Part: Right, Percent: 62},
		{ID: "00:00:00:00:00:a0/3", Name: "Earbuds", Kind: Headphones, Part: Case, Percent: 0},
		{ID: "00:00:00:00:00:a2/0", Name: "Headset", Kind: Headphones, Percent: UnknownPercent},
		{ID: "00:00:00:00:00:a1/0", Name: "Mouse", Kind: Mouse, Percent: 68},
	}
	if got := parseBluetoothBatteries(output, ioreg); !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got := parseBluetoothBatteries([]byte(`{"SPBluetoothDataType":[{}]}`), ioreg); len(got) != 0 {
		t.Fatalf("got readings for disconnected devices: %+v", got)
	}
}
