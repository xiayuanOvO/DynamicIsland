package nowplaying

import (
	"testing"
	"time"
)

func TestSleepWakeDetected(t *testing.T) {
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	if sleepWakeDetected(time.Time{}, base, 30*time.Second) {
		t.Fatal("zero previous time should not trigger sleep/wake detection")
	}
	if sleepWakeDetected(base, base.Add(30*time.Second), 30*time.Second) {
		t.Fatal("normal tick interval should not trigger sleep/wake detection")
	}
	if sleepWakeDetected(base, base.Add(44*time.Second), 30*time.Second) {
		t.Fatal("small jitter within margin should not trigger sleep/wake detection")
	}
	if !sleepWakeDetected(base, base.Add(46*time.Second), 30*time.Second) {
		t.Fatal("wall-clock jump beyond expected+margin should trigger sleep/wake detection")
	}
}
