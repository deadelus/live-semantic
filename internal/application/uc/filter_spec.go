package uc

import (
	"fmt"
	"strconv"
	"strings"

	"live-semantic/internal/domain/entities"
)

// filterTerm is one parsed "label" or "label*cap" component of a
// recognition filter spec (TODO.md § A, decision 2026-08-11: label-based
// filtering replaced the CLIP semantic gate — see docs/adr/clip-backend.md
// § 12).
type filterTerm struct {
	// Label is a YOLO11s/COCO class name (entities.Yolo11sClasses()),
	// already normalized (trim+lowercase, normalizeFilter).
	Label string
	// Cap is the maximum number of simultaneous tracks accepted for this
	// label — a "scene condition" (TODO.md § A/I): 1 by default ("person"
	// means "I expect a person box"), explicit with "*N" ("person*2" means
	// "up to 2 person boxes"). Enforced in trackManager.reanchor via
	// countByLabel — never blocks re-anchoring an *existing* track, only
	// spawning a *new* one once the cap is already reached.
	Cap int
}

// cocoLabels is the set of valid YOLO11s/COCO class names, built once from
// entities.Yolo11sClasses() — used to reject a typo'd filter label at parse
// time with a clear error, rather than silently detecting nothing (this
// project has hit that exact silent-failure shape before, see
// normalizeFilter's doc comment).
var cocoLabels = func() map[string]struct{} {
	classes := entities.Yolo11sClasses()
	set := make(map[string]struct{}, len(classes))
	for _, c := range classes {
		set[c] = struct{}{}
	}
	return set
}()

// parseFilterSpec parses a comma-separated filter spec into a set of
// filterTerm — "person" (cap 1, implicit), "person*2" (cap 2, explicit),
// "person*2,car" (two terms). Empty raw (after trimming) returns (nil, nil):
// "no filter", trackManager.acceptsLabel then accepts every label, matching
// the pre-2026-08-11 behavior of "no filter = track everything".
//
// Rejects (rather than silently ignoring):
//   - a label that isn't one of the 80 COCO classes YOLO11s can detect —
//     typo protection, not a real filter otherwise (would just never match
//     anything, indistinguishable from "detection is broken").
//   - a duplicate label across terms (e.g. "person*2,person*3") — the
//     caller (2026-08-11 decision) explicitly doesn't want overlapping
//     filter terms, since each is meant to be able to drive its own
//     event/action later (TODO.md § I) and a box can only have one label,
//     so a duplicate can never be a meaningful second condition, only a
//     mistake.
//   - a non-positive or non-numeric "*N".
func parseFilterSpec(raw string) ([]filterTerm, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	terms := make([]filterTerm, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty term in filter %q (stray comma?)", raw)
		}

		label, capStr, hasCap := strings.Cut(part, "*")
		label = normalizeFilter(label)
		if label == "" {
			return nil, fmt.Errorf("empty label in filter term %q", part)
		}

		capValue := 1
		if hasCap {
			capStr = strings.TrimSpace(capStr)
			n, err := strconv.Atoi(capStr)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid count %q in filter term %q: must be a positive integer", capStr, part)
			}
			capValue = n
		}

		if _, ok := cocoLabels[label]; !ok {
			return nil, fmt.Errorf("unknown label %q in filter term %q — not one of the 80 COCO classes YOLO11s can detect", label, part)
		}
		if _, dup := seen[label]; dup {
			return nil, fmt.Errorf("duplicate label %q in filter %q — each label can only appear once (no overlapping filter terms)", label, raw)
		}
		seen[label] = struct{}{}

		terms = append(terms, filterTerm{Label: label, Cap: capValue})
	}

	return terms, nil
}
