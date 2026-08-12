package uc

import (
	"fmt"
	"strconv"
	"strings"

	"live-semantic/internal/domain/entities"
)

// filterTerm is one parsed component of a recognition filter spec
// (TODO.md § A): "key[*cap][+option[=value]]...". key is either a
// YOLO11s/COCO class name (exact label match, no CLIP involved) or free
// text (matched semantically via CLIP against every YOLO candidate box
// not already claimed by an exact term this cycle) — trackManager decides
// which via isCOCOLabel(key), filterTerm itself doesn't know or care.
// Decision history: CLIP-only (2026-08-10) → label-only (2026-08-11
// morning) → hybrid (2026-08-11 afternoon) → this "+option" syntax
// (2026-08-11 evening, docs/adr/clip-backend.md § 12-13/16) — exact
// labels stay exact (no threshold fragility for the 80 classes YOLO
// already knows by name), free text gets CLIP back for anything outside
// that vocabulary, and per-term options (currently just Overlap) extend
// without needing a new delimiter each time.
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
	// Overlap allows this term to claim a candidate box already claimed by
	// another term this cycle — "+overlap" (implicit true) or
	// "+overlap=false"/"+overlap=true" (explicit). Default false: the
	// user confirmed 2026-08-11, tested live, that a term should never
	// draw a second box on the same physical object unless explicitly
	// asked to (docs/adr/clip-backend.md § 13/16). Parsed and stored here,
	// but NOT YET consulted by trackManager.reanchor — TODO.md § A tracks
	// wiring this in; setting it today has no observable effect.
	Overlap bool
	// Relation, RelationParam, Container, Attachment are set only for a
	// relational term: "container%relation[=param]%attachment" (e.g.
	// "person%+%backpack" — TODO.md § A "décomposition géométrique",
	// docs/adr/clip-backend.md § 24). Relation == "" (the vast majority of
	// terms) means every other field here is irrelevant — check that
	// first. Container/Attachment are matched independently by
	// trackManager (v1: both must be exact COCO labels — no CLIP/
	// fine-tuned fallback yet, docs/adr/clip-backend.md § 24's "sinon
	// repli sur sous-région heuristique" is future work), combined via
	// Relation's geometric check instead of a single CLIP score on a
	// composed phrase. Key holds the full canonical relation string
	// ("person%+%backpack") for identity/dedup/display — Container alone
	// ("person") for matching logic.
	Relation string
	// RelationParam holds the operator's parameter, parsed to its numeric
	// value already (0 for "+", which takes none) — e.g. the pixel
	// distance threshold for "near=50". Kept as a plain float32 rather
	// than a string: every current/planned operator's parameter is
	// numeric, and validating/parsing it once here (parseFilterSpec)
	// rather than deferring to trackManager keeps the same fail-fast
	// discipline as Cap's int parsing right above.
	RelationParam float32
	Container     string
	Attachment    string
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

// semanticLabelHint scans a semantic term's free-text key for exactly one
// COCO class name mentioned as a whole word/phrase (e.g. "person with a
// yellow hat" mentions "person") — trackManager uses this, when found, to
// restrict that term's CLIP candidates to boxes YOLO already labeled with
// the hinted class, instead of scoring every detected box regardless of
// its own label.
//
// Found necessary 2026-08-11, tested live: without this, a query like
// "person with a yellow hat" was scored against every candidate in the
// frame — including a couch and a potted plant, which sometimes outscored
// the actual (correctly detected, YOLO-labeled "person") person, because
// nothing in the semantic pass restricted candidates to plausible ones
// first (docs/adr/clip-backend.md § 13/17). Restricting to the mentioned
// class both fixes that (a couch is never even considered for a
// "person..." query) and is cheaper (fewer EncodeImage calls per cycle).
//
// Deliberately conservative: returns ("", false) — no restriction, every
// box still scored, matching the pre-existing behavior — unless EXACTLY
// one COCO class name appears in key. Zero matches means the term
// genuinely doesn't reference a known class (nothing to restrict to).
// More than one is ambiguous (e.g. "person near a car" mentions both) —
// guessing which one is the real subject risks silently restricting to
// the wrong class, worse than not restricting at all.
func semanticLabelHint(key string) (string, bool) {
	var found []string
	for label := range cocoLabels {
		if containsWord(key, label) {
			found = append(found, label)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

// containsWord reports whether needle appears in haystack bounded by
// non-alphanumeric characters (or string edges) on both sides — a plain
// strings.Contains would let "car" match inside "scary" or "carpet", which
// semanticLabelHint must not do. ASCII-only word-boundary notion, matching
// the ASCII-only COCO class names this is checked against.
func containsWord(haystack, needle string) bool {
	for start := 0; ; {
		idx := strings.Index(haystack[start:], needle)
		if idx == -1 {
			return false
		}
		idx += start
		end := idx + len(needle)

		beforeOK := idx == 0 || !isWordByte(haystack[idx-1])
		afterOK := end == len(haystack) || !isWordByte(haystack[end])
		if beforeOK && afterOK {
			return true
		}
		start = idx + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// splitRelation extracts a leading "container%relation[=param]%" prefix
// from part, if present — docs/adr/clip-backend.md § 24's
// "term := key ['%' operateur ['=' valeur] '%' key] [...]" grammar.
// Returns hasRelation=false (container/relation/param zeroed, rest==part
// unchanged) when part contains no '%' at all — the overwhelming majority
// of terms, which never touch this function's parsing beyond the initial
// index scan.
//
// Deliberately run BEFORE any '+'-based splitting (parseFilterSpec calls
// this first): the one relation operator implemented so far is literally
// named "+" (%+%, chosen 2026-08-11 for containment) — splitting on '+'
// first, as every other part of this grammar already does for options,
// would shred "%+%" into fragments ("person%", "%backpack...") and misread
// the relation as two bogus '+' options. Extracting the relation first,
// with its own '%'-delimited scan, sidesteps that collision entirely
// rather than special-casing '+' inside the option splitter.
func splitRelation(part string) (container, relation, param, rest string, hasRelation bool, err error) {
	firstPct := strings.Index(part, "%")
	if firstPct == -1 {
		return "", "", "", part, false, nil
	}
	container = strings.TrimSpace(part[:firstPct])
	if container == "" {
		return "", "", "", "", false, fmt.Errorf("empty container before '%%' in filter term %q", part)
	}

	afterFirst := part[firstPct+1:]
	secondPct := strings.Index(afterFirst, "%")
	if secondPct == -1 {
		return "", "", "", "", false, fmt.Errorf("unterminated relation operator in filter term %q (expected a second '%%')", part)
	}
	opAndParam := afterFirst[:secondPct]
	rest = afterFirst[secondPct+1:]
	if rest == "" {
		return "", "", "", "", false, fmt.Errorf("missing attachment after relation operator in filter term %q", part)
	}

	relation, param, _ = strings.Cut(opAndParam, "=")
	relation = strings.TrimSpace(strings.ToLower(relation))
	param = strings.TrimSpace(param)
	if relation == "" {
		return "", "", "", "", false, fmt.Errorf("empty relation operator in filter term %q", part)
	}
	return container, relation, param, rest, true, nil
}

// relationOperatorSpec describes one recognized relation operator's
// parameter shape — requiresParam distinguishes "+" (containment, no
// parameter — relationContainmentThreshold, tracking.go, is a fixed
// constant, not per-term yet) from "near" (proximity, requires a pixel
// distance — there's no sane fixed default the way there is for
// containment, docs/adr/clip-backend.md § 24 always described "near" as
// needing its own parameter).
type relationOperatorSpec struct {
	requiresParam bool
}

// relationOperators is the closed set of recognized relation operators —
// deliberately small (docs/adr/clip-backend.md § 24/28: "+" containment
// and "near" proximity, others intentionally deferred until there's a
// real need). An unrecognized name errors clearly rather than silently
// matching nothing, same philosophy as "+option" (filterTerm's doc
// comment).
var relationOperators = map[string]relationOperatorSpec{
	"+":    {requiresParam: false},
	"near": {requiresParam: true},
}

// parseFilterSpec parses a comma-separated filter spec into a set of
// filterTerm:
//
//	person                              -> Key="person" Cap=1 Overlap=false
//	person*2                            -> Cap=2
//	person with a red hat*1             -> Key="person with a red hat" (semantic, not a COCO label)
//	person with a red hat*1+overlap     -> Overlap=true
//	person with a red hat*1+overlap=false (equivalent to omitting +overlap)
//	person*2,car                        -> two independent exact terms
//	person%+%backpack                   -> Relation="+" Container="person" Attachment="backpack" Key="person%+%backpack"
//	person%+%backpack*2+overlap         -> cap/options apply to the whole relation, not "backpack" alone
//	person%near=50%car                  -> Relation="near" RelationParam=50 (pixel gap threshold)
//
// Empty raw (after trimming) returns (nil, nil): "no filter",
// trackManager then accepts every label and never calls CLIP.
//
// Grammar per comma-separated term: "key[*cap][+option[=value]]...". "*"
// must come first if present, then any number of "+option"/"+option=value"
// segments — "+" is reserved (not usable inside a free-text key) once this
// grammar applies.
//
// Rejects (rather than silently ignoring):
//   - a duplicate key across terms (e.g. "person*2,person*3", or the same
//     free-text term twice) — each term is meant to be able to drive its
//     own event/action later (TODO.md § A/I) and a candidate box can only
//     be claimed by one term per cycle by default (Overlap=false), so a
//     duplicate can never be a meaningful second condition, only a
//     mistake.
//   - a non-positive or non-numeric "*N".
//   - an unrecognized "+option" name, or a value that isn't a valid bool
//     for "+overlap[=value]" — the option grammar is a closed set (unlike
//     the key, which is deliberately open per the semantic-term decision
//     above), so a typo here has no reasonable fallback interpretation.
//   - the same option repeated within one term (e.g. "...+overlap+overlap").
//
// Does NOT reject an unknown (non-COCO) key — that used to be an error
// (2026-08-11 morning decision) but is now a valid semantic term. A
// genuine typo of a COCO label (e.g. "pesron") silently becomes a
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

		// Relation extraction must happen before any '+'-based splitting —
		// see splitRelation's doc comment (the "%+%" operator's own name
		// collides with the '+' option delimiter otherwise).
		container, relation, param, rest, hasRelation, err := splitRelation(part)
		if err != nil {
			return nil, err
		}

		splitTarget := part
		if hasRelation {
			splitTarget = rest // "backpack*2+overlap" — container/relation already consumed
		}
		segments := strings.Split(splitTarget, "+")
		mainSegment, optionSegments := segments[0], segments[1:]

		key, capStr, hasCap := strings.Cut(mainSegment, "*")
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

		var term filterTerm
		if hasRelation {
			spec, ok := relationOperators[relation]
			if !ok {
				return nil, fmt.Errorf("unknown relation operator %q in filter term %q", relation, part)
			}
			var paramValue float32
			switch {
			case spec.requiresParam && param == "":
				return nil, fmt.Errorf("relation operator %q in filter term %q requires a parameter (e.g. \"%s=50\")", relation, part, relation)
			case !spec.requiresParam && param != "":
				return nil, fmt.Errorf("relation operator %q in filter term %q doesn't take a parameter, got %q", relation, part, param)
			case spec.requiresParam:
				v, err := strconv.ParseFloat(param, 32)
				if err != nil || v <= 0 {
					return nil, fmt.Errorf("invalid parameter %q for relation operator %q in filter term %q: must be a positive number", param, relation, part)
				}
				paramValue = float32(v)
			}

			container = normalizeFilter(container)
			if !isCOCOLabel(container) {
				return nil, fmt.Errorf("relation container %q in filter term %q must be a known COCO class (v1: no semantic/fine-tuned container yet)", container, part)
			}
			if !isCOCOLabel(key) {
				return nil, fmt.Errorf("relation attachment %q in filter term %q must be a known COCO class (v1: no semantic/fine-tuned attachment yet)", key, part)
			}
			if container == key {
				return nil, fmt.Errorf("relation container and attachment are both %q in filter term %q — a box can't be its own attachment", container, part)
			}
			canonical := container + "%" + relation
			if param != "" {
				canonical += "=" + param
			}
			canonical += "%" + key
			term = filterTerm{
				Key: canonical, Cap: capValue,
				Relation: relation, RelationParam: paramValue,
				Container: container, Attachment: key,
			}
		} else {
			term = filterTerm{Key: key, Cap: capValue}
		}
		seenOptions := make(map[string]struct{}, len(optionSegments))
		for _, optSeg := range optionSegments {
			name, valueStr, hasValue := strings.Cut(optSeg, "=")
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" {
				return nil, fmt.Errorf("empty option in filter term %q", part)
			}
			if _, dup := seenOptions[name]; dup {
				return nil, fmt.Errorf("option %q repeated in filter term %q", name, part)
			}
			seenOptions[name] = struct{}{}

			switch name {
			case "overlap":
				value := true // "+overlap" alone means true
				if hasValue {
					v, err := strconv.ParseBool(strings.TrimSpace(valueStr))
					if err != nil {
						return nil, fmt.Errorf("invalid value %q for option %q in filter term %q: must be a boolean", valueStr, name, part)
					}
					value = v
				}
				term.Overlap = value
			default:
				return nil, fmt.Errorf("unknown option %q in filter term %q", name, part)
			}
		}

		// term.Key (not the bare "key" var above) — for a relational term
		// that's the canonical "container%relation%attachment" string, not
		// just the attachment half, which alone wouldn't be a meaningful
		// dedup key (two different relations could legitimately share the
		// same attachment, e.g. "person%+%backpack,car%+%backpack").
		if _, dup := seen[term.Key]; dup {
			return nil, fmt.Errorf("duplicate term %q in filter %q — each term can only appear once (no overlapping filter terms)", term.Key, raw)
		}
		seen[term.Key] = struct{}{}

		terms = append(terms, term)
	}

	return terms, nil
}
