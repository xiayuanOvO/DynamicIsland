package battery

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func Read(ctx context.Context) []Reading {
	var readings []Reading
	if output, err := exec.CommandContext(ctx, "/usr/bin/pmset", "-g", "batt").Output(); err == nil {
		if computer, ok := parseMacBattery(string(output)); ok {
			readings = append(readings, computer)
		}
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "/usr/sbin/system_profiler", "SPBluetoothDataType", "-json").Output()
	if err != nil {
		return readings
	}
	registry, _ := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-a", "-r", "-l", "-k", "BatteryPercent", "-d", "1").Output()
	connected := parseBluetoothBatteries(output, registryBatteries(registry))
	targets := bluetoothProbeTargets(connected)
	if len(targets) == 0 {
		return append(readings, connected...)
	}
	resources, err := mygo.App.Path(mygo.PathResources)
	if err == nil {
		if output, err := exec.CommandContext(ctx, filepath.Join(resources, "bluetooth-battery"), slices.Sorted(maps.Keys(targets))...).Output(); err == nil {
			connected = mergeBluetoothProbe(connected, targets, output)
		}
	}
	return append(readings, connected...)
}

func parseMacBattery(output string) (Reading, bool) {
	for line := range strings.SplitSeq(output, "\n") {
		name, status, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found || !strings.HasPrefix(name, "-InternalBattery-") || strings.Contains(status, "present: false") {
			continue
		}
		level, status, _ := strings.Cut(status, ";")
		percent, ok := batteryPercent(level)
		if ok {
			state, _, _ := strings.Cut(status, ";")
			return Reading{ID: "computer", Name: "本机", Kind: Computer, Percent: percent, Charging: strings.TrimSpace(state) == "charging"}, true
		}
	}
	return Reading{}, false
}

func parseBluetoothBatteries(output []byte, registry map[string]int) []Reading {
	type device struct {
		Address string          `json:"device_address"`
		Type    string          `json:"device_minorType"`
		Main    json.RawMessage `json:"device_batteryLevelMain"`
		Left    json.RawMessage `json:"device_batteryLevelLeft"`
		Right   json.RawMessage `json:"device_batteryLevelRight"`
		Case    json.RawMessage `json:"device_batteryLevelCase"`
	}
	var report struct {
		Controllers []struct {
			Connected []map[string]device `json:"device_connected"`
		} `json:"SPBluetoothDataType"`
	}
	if json.Unmarshal(output, &report) != nil {
		return nil
	}
	var readings []Reading
	seen := make(map[string]bool)
	for _, controller := range report.Controllers {
		for _, entry := range controller.Connected {
			for name, device := range entry {
				address := bluetoothAddress(device.Address)
				id := address
				if id == "" {
					id = name
				}
				kind := Other
				switch strings.ToLower(device.Type) {
				case "mouse":
					kind = Mouse
				case "keyboard":
					kind = Keyboard
				case "trackpad":
					kind = Trackpad
				case "headphones", "headset":
					kind = Headphones
				}
				left, hasLeft := batteryPercent(string(device.Left))
				right, hasRight := batteryPercent(string(device.Right))
				casePercent, hasCase := batteryPercent(string(device.Case))
				main, hasMain := batteryPercent(string(device.Main))
				if !hasMain {
					main, hasMain = registry[address]
				}
				if !hasMain {
					main = UnknownPercent
				}
				for _, part := range []struct {
					part    Part
					percent int
					present bool
				}{
					{Whole, main, !hasLeft && !hasRight && (hasMain || !hasCase)},
					{Left, left, hasLeft},
					{Right, right, hasRight},
					{Case, casePercent, hasCase},
				} {
					key := id + "/" + strconv.Itoa(int(part.part))
					if !part.present || seen[key] {
						continue
					}
					seen[key] = true
					readings = append(readings, Reading{ID: key, Name: name, Kind: kind, Part: part.part, Percent: part.percent})
				}
			}
		}
	}
	slices.SortFunc(readings, func(a, b Reading) int {
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return readings
}

func registryBatteries(output []byte) map[string]int {
	var plist struct {
		Entries []struct {
			Fields []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"array>dict"`
	}
	if xml.Unmarshal(output, &plist) != nil {
		return nil
	}
	levels := make(map[string]int)
	for _, entry := range plist.Entries {
		var address string
		var percent int
		var ok bool
		for i := 0; i+1 < len(entry.Fields); i += 2 {
			key, value := entry.Fields[i], entry.Fields[i+1]
			switch {
			case key.Value == "DeviceAddress" && value.XMLName.Local == "string":
				address = bluetoothAddress(value.Value)
			case key.Value == "BatteryPercent" && value.XMLName.Local == "integer":
				percent, ok = batteryPercent(value.Value)
			}
		}
		if address != "" && ok {
			levels[address] = percent
		}
	}
	return levels
}

func bluetoothAddress(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", ":")
}

func batteryPercent(value string) (int, bool) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	percent, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(value, "%")))
	return percent, err == nil && percent >= 0 && percent <= 100
}
