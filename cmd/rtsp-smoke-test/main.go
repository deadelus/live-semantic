// Command rtsp-smoke-test is a small dev tool (not part of the shipped
// application) to validate implementation/streamer/input.FileInput against
// a real RTSP session — TODO.md § H1 / docs/gui/spec.md § 4 risk #3 called
// for this to be verified end-to-end, not just assumed to work because
// FFmpeg is present in the OpenCV build.
//
// Unlike cmd/tracking-drift, this one is kept in the repo rather than
// deleted after a single use: RTSP is the kind of thing that's cheap to
// regress silently (a gocv/FFmpeg upgrade, a URI-handling refactor in
// FileInput) and there's no automated test for it — see FileInput's doc
// comment for why (would need a vendored RTSP server or a live network
// dependency, neither fits this project's test conventions). Rerunning
// this tool is the fastest way to notice a regression, not just the tool
// used once to write TODO.md's checkmark.
//
// No public demo RTSP stream is reliable enough to depend on (they rotate
// or go offline). Self-host one instead:
//
//	brew install mediamtx
//	mediamtx &
//	ffmpeg -re -stream_loop -1 -i assets/videos/car.mp4 -c copy -f rtsp rtsp://127.0.0.1:8554/car &
//	go run ./cmd/rtsp-smoke-test
//	# or against any other RTSP source:
//	go run ./cmd/rtsp-smoke-test -uri rtsp://user:pass@camera.local:554/stream
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/streamer/input"
)

func main() {
	uri := flag.String("uri", "rtsp://127.0.0.1:8554/car", "RTSP (or any FileInput-compatible) URI to read from")
	duration := flag.Duration("duration", 5*time.Second, "how long to read frames before stopping")
	logEvery := flag.Int("log-every", 30, "print progress every N frames (plus the first 3)")
	flag.Parse()

	fi := input.NewFileInput(*uri)

	initStart := time.Now()
	if err := fi.Initialize(); err != nil {
		fmt.Println("Initialize error:", err)
		os.Exit(1)
	}
	fmt.Printf("Initialize(%q) took %s\n", *uri, time.Since(initStart))

	var count int
	streamStart := time.Now()
	deadline := streamStart.Add(*duration)

	err := fi.Start(func(f *entities.Frame) (*entities.Frame, error) {
		count++
		if count <= 3 || count%*logEvery == 0 {
			b := f.Image.Bounds()
			fmt.Printf("frame %d: %dx%d at %s\n", f.FrameNumber, b.Dx(), b.Dy(), time.Now().Format("15:04:05.000"))
		}
		if time.Now().After(deadline) {
			fi.Stop()
		}
		return f, nil
	})
	if err != nil {
		fmt.Println("Start error:", err)
		os.Exit(1)
	}

	elapsed := time.Since(streamStart)
	fmt.Printf("total frames read: %d over %s (%.1f fps)\n", count, elapsed, float64(count)/elapsed.Seconds())
	if count == 0 {
		fmt.Println("FAIL: read 0 frames — is the RTSP server actually publishing? (see this file's doc comment for how to self-host one)")
		os.Exit(1)
	}
}
