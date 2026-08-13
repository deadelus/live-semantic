package input

import "gocv.io/x/gocv"

// ProbeDevices tries opening each camera device index in [0, maxIndex)
// just long enough to check gocv.VideoCapture.IsOpened(), then closes it
// immediately — there is no portable OS-level "list connected webcams"
// API reachable from gocv/OpenCV, so index-probing (the same index
// scheme NewCameraInput already uses) is the pragmatic substitute, not a
// true enumeration: no device name/model, and a USB camera can be
// unplugged between one probe and the next (index that opened once isn't
// guaranteed to keep working).
//
// skipIndices marks indices the caller already knows are real and busy
// (a running session's device) — those are trusted as-is rather than
// re-opened: this same process opening a device it already holds
// exclusively elsewhere would at best fail spuriously, at worst
// interfere with an active stream. Added 2026-08-13 alongside the
// device-picker GUI (TODO.md § H2) — direct follow-up to the earlier
// "two sources silently fighting over the same webcam" bug fix.
func ProbeDevices(maxIndex int, skipIndices map[int]bool) []int {
	available := make([]int, 0, maxIndex)
	for i := 0; i < maxIndex; i++ {
		if skipIndices[i] {
			available = append(available, i)
			continue
		}
		cap, err := gocv.OpenVideoCapture(i)
		if cap != nil {
			if err == nil && cap.IsOpened() {
				available = append(available, i)
			}
			cap.Close()
		}
	}
	return available
}
