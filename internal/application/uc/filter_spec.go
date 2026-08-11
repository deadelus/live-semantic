package uc

import (
	"fmt"
	"strconv"
	"strings"

	"live-semantic/internal/domain/entities"
)

// filterTerm is one parsed "key" or "key*cap" component of a recognition
// filter spec (TODO.md § A). key is either a YOLO11s/COCO class name
// (exact label match, no CLIP involved) or free text (matched
// semantically via CLIP against every YOLO candidate box not already
// claimed by an exact term this cycle) — trackManager decides which via
// isCOCOLabel(key), filterTerm itself doesn't know or care. Decision
// history: CLIP-only (2026-08-10) → label-only (2026-08-11 morning) →
// this hybrid (2026-08-11 afternoon, docs/adr/clip-backend.md § 12-13) —
// exact labels stay exact (no threshold fragility for the 80 classes
// YOLO already knows by name), free text gets CLIP back for anything
// outside that vocabulary.
type filterTerm struct {
	// Key is the term's identity: a COCO label for an exact match, or the
	// raw (trimmed/lowercased) free text for a semantic match. Doubles as
	// the cap-counting bucket (trackManager.countByFilterKey) and the
	// dedup key below.
	Key string
	// Cap is the maximum number of simultaneous tracks accepted for this
	// term — a "scene condition" (TODO.md § A/I): 1 by default ("person"
	// means "I expect a person box"), explicit with "*N" ("person*2" means
	// "up to 2 person boxes"). For a semantic term, candidates are ranked
	// by CLIP score and only the top Cap above threshold are kept. Never
	// blocks re-anchoring an *existing* track, only spawning a *new* one
	// once the cap is already reached.
	Cap int
}

// cocoLabels is the set of valid YOLO11s/COCO class names, built once from
// entities.Yolo11sClasses().
var cocoLabels = func() map[string]struct{} {
	classes := entities.Yolo11sClasses()
	set := make(map[string]struct{}, len(classes))
	for _, c := range classes {
		set[c] = struct{}{}
	}
	return set
}()

// isCOCOLabel reports whether key is one of the 80 COCO classes YOLO11s
// can detect — trackManager uses this to decide whether a term matches by
// exact label (true) or semantically via CLIP (false).
func isCOCOLabel(key string) bool {
	_, ok := cocoLabels[key]
	return ok
}

// parseFilterSpec parses a comma-separated filter spec into a set of
// filterTerm — "person" (cap 1, implicit, exact — a COCO label), "person*2"
// (cap 2, exact), "person with a red hat*1" (cap 1, semantic — not a COCO
// label), "person*2,car" (two exact terms). Empty raw (after trimming)
// returns (nil, nil): "no filter", trackManager then accepts every label
// and never calls CLIP.
//
// Rejects (rather than silently ignoring):
//   - a duplicate key across terms (e.g. "person*2,person*3", or the same
//     free-text term twice) — each term is meant to be able to drive its
//     own event/action later (TODO.md § A/I) and a candidate box can only
//     be claimed by one term per cycle (trackManager.reanchor), so a
//     duplicate can never be a meaningful second condition, only a
//     mistake.
//   - a non-positive or non-numeric "*N".
//
// Does NOT reject an unknown (non-COCO) key anymore — that used to be an
// error (2026-08-11 morning decision) but is now a valid semantic term.
// A genuine typo of a COCO label (e.g. "pesron") silently becomes a
// (probably low-scoring) semantic filter instead of erroring — an
// accepted tradeoff of opening up free text again, not re-litigated here.
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

		key, capStr, hasCap := strings.Cut(part, "*")
		key = normalizeFilter(key)
		if key == "" {
			return nil, fmt.Errorf("empty term before '*' in filter term %q", part)
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

		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate term %q in filter %q — each term can only appear once (no overlapping filter terms)", key, raw)
		}
		seen[key] = struct{}{}

		terms = append(terms, filterTerm{Key: key, Cap: capValue})
	}

	return terms, nil
}
