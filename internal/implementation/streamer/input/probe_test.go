package input

import "testing"

func TestProbeDevices_ZeroRangeReturnsEmpty(t *testing.T) {
	got := ProbeDevices(0, nil)
	if len(got) != 0 {
		t.Fatalf("ProbeDevices(0, nil) = %v, want empty", got)
	}
}

// Deliberately doesn't assert on indices *other* than the skipped one —
// whether device 0/2 are "available" depends on real hardware attached
// to whatever machine runs this test (unlike camera_test.go's other
// cases, found the hard way: this repo's own dev machine has a real
// camera at index 0). What's portable to assert: a skipped index is
// always trusted and returned without ever being probed.
func TestProbeDevices_TrustsSkippedIndicesWithoutProbing(t *testing.T) {
	got := ProbeDevices(3, map[int]bool{1: true})
	for _, idx := range got {
		if idx == 1 {
			return
		}
	}
	t.Fatalf("ProbeDevices(3, {1:true}) = %v, want it to contain skipped index 1", got)
}
