// Command tracking-drift-bench is a dev tool (not part of the shipped
// application) to run the drift test called for in TODO.md § B: compare
// KCF vs CSRT (docs/adr/object-tracking.md) on real footage, headless (no
// window — this environment has no display).
//
// Named -bench, not -smoke-test (cf. cmd/rtsp-smoke-test): this one
// produces a quality metric (IoU drift) to compare two algorithms and
// inform a decision, not a pass/fail connectivity check. Kept in the repo
// after that decision was made (KCF, docs/adr/object-tracking.md § 7-8) in
// case it's ever revisited with new footage or a third algorithm — most
// other one-off dev tools in this project's history were deleted once
// their result was written down (see TODO.md for several).
//
// Methodology: track a single object across the video using the tracker
// alone, re-anchoring against a fresh YOLO detection every -interval
// frames (mirrors internal/application/uc/tracking.go's reanchor/advance
// split). At each re-anchor checkpoint, measure the IoU between the
// tracker's belief (drifted since the last checkpoint) and the fresh
// detection *before* resetting the tracker on it — that IoU is the drift
// metric. Also counts raw tracker.Update() failures between checkpoints.
//
// Usage:
//
//	go run ./cmd/tracking-drift-bench -video assets/videos/car.mp4 -class car
//	go run ./cmd/tracking-drift-bench -video assets/videos/person.mp4 -class person
package main

import (
	"flag"
	"fmt"
	"os"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/implementation/inference/onnx/yolo11s"
	gocvtracker "live-semantic/internal/implementation/tracking/gocv-tracker"
	"live-semantic/internal/infrastructure/inference"

	"gocv.io/x/gocv"
)

type report struct {
	algorithm            string
	totalFrames          int
	checkpoints          int
	ious                 []float32
	trackerFailures      int
	unmatchedCheckpoints int
}

func (r report) print() {
	if len(r.ious) == 0 {
		fmt.Printf("%-6s frames=%-4d checkpoints=%-3d  NO IoU SAMPLE (target lost immediately or never re-detected)  tracker_failures=%d unmatched_checkpoints=%d\n",
			r.algorithm, r.totalFrames, r.checkpoints, r.trackerFailures, r.unmatchedCheckpoints)
		return
	}

	var sum, min, max float32
	min, max = r.ious[0], r.ious[0]
	for _, v := range r.ious {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	avg := sum / float32(len(r.ious))

	fmt.Printf("%-6s frames=%-4d checkpoints=%-3d avg_iou=%.3f min_iou=%.3f max_iou=%.3f tracker_failures=%d unmatched_checkpoints=%d\n",
		r.algorithm, r.totalFrames, r.checkpoints, avg, min, max, r.trackerFailures, r.unmatchedCheckpoints)
}

func matToFrame(mat *gocv.Mat, frameNumber uint64) (*entities.Frame, error) {
	img, err := mat.ToImage()
	if err != nil {
		return nil, err
	}
	return &entities.Frame{Image: img, FrameNumber: frameNumber}, nil
}

// pickTarget returns the box to track from a detection result: the first
// box matching classFilter (if set), otherwise the highest-confidence box.
func pickTarget(boxes []entities.BoundingBox, classFilter string) (entities.BoundingBox, bool) {
	best := entities.BoundingBox{}
	found := false

	for _, b := range boxes {
		if classFilter != "" && b.Label != classFilter {
			continue
		}
		if !found || b.Confidence > best.Confidence {
			best = b
			found = true
		}
	}

	return best, found
}

func runDriftTest(videoPath string, algo gocvtracker.Algorithm, detector inference.ObjectDetector, interval int, classFilter string) (report, error) {
	rep := report{algorithm: algo.String()}

	vc, err := gocv.VideoCaptureFile(videoPath)
	if err != nil {
		return rep, fmt.Errorf("open video: %w", err)
	}
	defer vc.Close()

	mat := gocv.NewMat()
	defer mat.Close()

	// The target class isn't necessarily visible on frame 0 (e.g. car.mp4:
	// no car in frame until frame ~36) — scan forward for the first frame
	// with a matching detection instead of assuming frame 0.
	const maxSeekFrames = 300
	var frame0 *entities.Frame
	var target entities.BoundingBox
	frameIdx := -1

	for frameIdx < maxSeekFrames {
		if ok := vc.Read(&mat); !ok || mat.Empty() {
			return rep, fmt.Errorf("reached end of video without a matching detection (class filter %q)", classFilter)
		}
		frameIdx++
		rep.totalFrames++

		f, err := matToFrame(&mat, uint64(frameIdx))
		if err != nil {
			continue
		}

		result, err := detector.AnalyzeFrame(f)
		if err != nil {
			return rep, fmt.Errorf("detection at frame %d: %w", frameIdx, err)
		}
		if box, found := pickTarget(result.BoundingBoxes, classFilter); found {
			frame0, target = f, box
			break
		}
	}
	if frame0 == nil {
		return rep, fmt.Errorf("no matching object detected in the first %d frames (class filter %q)", maxSeekFrames, classFilter)
	}

	trk, err := gocvtracker.New(algo)
	if err != nil {
		return rep, fmt.Errorf("new tracker: %w", err)
	}
	defer trk.Cleanup()

	if err := trk.Init(frame0, target); err != nil {
		return rep, fmt.Errorf("tracker init: %w", err)
	}

	current := target
	checkpointBase := frameIdx

	for {
		if ok := vc.Read(&mat); !ok || mat.Empty() {
			break
		}
		frameIdx++
		rep.totalFrames++

		frame, err := matToFrame(&mat, uint64(frameIdx))
		if err != nil {
			continue
		}

		if (frameIdx-checkpointBase)%interval == 0 {
			rep.checkpoints++
			result, err := detector.AnalyzeFrame(frame)
			if err != nil {
				rep.unmatchedCheckpoints++
				continue
			}

			groundTruth, found := bestSameClassMatch(current, result.BoundingBoxes)
			if !found {
				rep.unmatchedCheckpoints++
				continue
			}

			iou := current.IoU(&groundTruth)
			rep.ious = append(rep.ious, iou)

			// Re-anchor, mirroring production behaviour (tracking.go's
			// reanchor()): reset the tracker on ground truth.
			current = groundTruth
			if err := trk.Init(frame, current); err != nil {
				return rep, fmt.Errorf("tracker re-init at frame %d: %w", frameIdx, err)
			}
			continue
		}

		box, ok := trk.Update(frame)
		if !ok {
			rep.trackerFailures++
			continue
		}
		current = box
	}

	return rep, nil
}

// bestSameClassMatch finds, among boxes, the one with the same class as
// current with the highest IoU against it (even if that IoU is 0 — a total
// miss is itself a meaningful drift data point, not something to discard).
func bestSameClassMatch(current entities.BoundingBox, boxes []entities.BoundingBox) (entities.BoundingBox, bool) {
	best := entities.BoundingBox{}
	bestIoU := float32(-1)
	found := false

	for _, b := range boxes {
		if b.Label != current.Label {
			continue
		}
		iou := current.IoU(&b)
		if iou > bestIoU {
			bestIoU = iou
			best = b
			found = true
		}
	}

	return best, found
}

func main() {
	videoPath := flag.String("video", "", "path to the video file to test (required)")
	classFilter := flag.String("class", "", "COCO class label to track (default: highest-confidence detection on frame 0)")
	interval := flag.Int("interval", 45, "frames between re-anchoring checkpoints (matches production reanchorInterval by default)")
	flag.Parse()

	if *videoPath == "" {
		fmt.Fprintln(os.Stderr, "usage: tracking-drift-bench -video <path> [-class <label>] [-interval <n>]")
		os.Exit(2)
	}

	detector, err := yolo11s.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "detector init failed:", err)
		os.Exit(1)
	}
	defer detector.Cleanup()

	fmt.Printf("=== %s (class filter: %q, interval: %d frames) ===\n", *videoPath, *classFilter, *interval)

	for _, algo := range []gocvtracker.Algorithm{gocvtracker.KCF, gocvtracker.CSRT} {
		rep, err := runDriftTest(*videoPath, algo, detector, *interval, *classFilter)
		if err != nil {
			fmt.Printf("%-6s ERROR: %v\n", algo.String(), err)
			continue
		}
		rep.print()
	}
}
