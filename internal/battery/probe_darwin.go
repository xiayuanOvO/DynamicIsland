package battery

import (
	"encoding/binary"
	"encoding/json"
	"maps"
	"strconv"
	"strings"
)

func bluetoothProbeTargets(connected []Reading) map[string]int {
	// CoreBluetooth has no public Bluetooth address. Match a device only
	// when its name is unique in both the system and CoreBluetooth reports.
	targets := make(map[string]int, len(connected))
	for i, reading := range connected {
		_, exists := targets[reading.Name]
		if exists || reading.Percent != UnknownPercent || reading.Name == "" {
			targets[reading.Name] = -1
		} else {
			targets[reading.Name] = i
		}
	}
	maps.DeleteFunc(targets, func(_ string, index int) bool { return index < 0 })
	return targets
}

func mergeBluetoothProbe(connected []Reading, targets map[string]int, output []byte) []Reading {
	type report struct {
		Name string `json:"name"`
		Oppo bool   `json:"oppo"`
		Data []byte `json:"data"`
	}
	var reports []report
	if json.Unmarshal(output, &reports) != nil || len(reports) == 0 {
		return connected
	}
	byIndex := make(map[int]report, len(reports))
	for _, report := range reports {
		if index, found := targets[report.Name]; found {
			byIndex[index] = report
		}
	}
	readings := make([]Reading, 0, len(connected))
	for i, reading := range connected {
		if report, found := byIndex[i]; found {
			if report.Oppo {
				if parts := encoBatteryParts(reading, report.Data); len(parts) != 0 {
					readings = append(readings, parts...)
					continue
				}
			} else if len(report.Data) == 1 && report.Data[0] <= 100 {
				reading.Percent = int(report.Data[0])
			}
		}
		readings = append(readings, reading)
	}
	return readings
}

func encoBatteryParts(device Reading, reply []byte) []Reading {
	// Enco Free4 replies with a single frame. Its payload contains a status
	// byte, a count, and pairs of component ID and encoded battery level.
	const header, payloadPrefix = 9, 2
	if len(reply) < header+payloadPrefix || int(reply[1]) != len(reply)-2 || reply[2]&3 != 0 ||
		int(binary.LittleEndian.Uint16(reply[7:9])) != len(reply)-header || reply[9] != 0 ||
		int(reply[10])*2 != len(reply)-header-payloadPrefix {
		return nil
	}
	id := strings.TrimSuffix(device.ID, "/0")
	var parts [Case + 1]Reading
	for i := header + payloadPrefix; i < len(reply); i += 2 {
		part, level := Part(reply[i]), reply[i+1]
		if part < Left || part > Case || level&0x7f > 100 {
			continue
		}
		reading := device
		reading.ID = id + "/" + strconv.Itoa(int(part))
		reading.Part = part
		reading.Percent = int(level & 0x7f)
		reading.Charging = level&0x80 != 0
		parts[part] = reading
	}
	var readings []Reading
	for _, reading := range parts {
		if reading.Part != Whole {
			readings = append(readings, reading)
		}
	}
	return readings
}
